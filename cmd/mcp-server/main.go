package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/aidanhall34/make-mcp/pkg/auth"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/logging"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/aidanhall34/make-mcp/pkg/resources"
	makecpserver "github.com/aidanhall34/make-mcp/pkg/server"
	"github.com/aidanhall34/make-mcp/pkg/server/transport"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
	"github.com/aidanhall34/make-mcp/pkg/watcher"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mcp-server", flag.ContinueOnError)

	var (
		configPath string
		delimiter  string
		makefiles  stringSlice
		transportf string
		listen     string
		logPath    string
		strict     bool
		debug      bool
	)

	fs.StringVar(&configPath, "config", "", "path to make-mcp.yml configuration file")
	fs.StringVar(&delimiter, "delimiter", "", "annotation delimiter")
	fs.Var(&makefiles, "makefile", "Makefile path to parse; may be repeated")
	fs.StringVar(&transportf, "transport", "", "transport to enable: stdio, http, or both")
	fs.StringVar(&listen, "listen", "", "HTTP listen address")
	fs.StringVar(&logPath, "log-path", "", "destination for JSON logs (e.g. stderr, or a file path)")
	fs.BoolVar(&strict, "strict", false, "require every supported recipe annotation, including optional MCP tool hints")
	fs.BoolVar(&debug, "debug", false, "enable debug logging (includes tool args, stdout, and stderr)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	cliOverride := config.Config{
		Delimiter: delimiter,
		Makefiles: []string(makefiles),
		Transport: transportf,
		Listen:    listen,
		LogPath:   logPath,
		Strict:    strict,
		Debug:     debug,
	}
	cfg, err := loadConfig(configPath, cliOverride)
	if err != nil {
		return err
	}
	if len(cfg.Makefiles) == 0 {
		return fmt.Errorf("no Makefiles specified; use --makefile <path> or set makefiles in make-mcp.yml")
	}
	if cfg.Transport != "stdio" && cfg.Transport != "http" && cfg.Transport != "both" {
		return fmt.Errorf("unsupported transport %q", cfg.Transport)
	}

	// Validate OAuth constraints.
	if cfg.OAuth.Enabled {
		if cfg.Transport == "stdio" {
			return fmt.Errorf("oauth is not compatible with stdio transport; use http or both")
		}
		if cfg.OAuth.TLS.Cert == "" || cfg.OAuth.TLS.Key == "" {
			return fmt.Errorf("oauth.tls.cert and oauth.tls.key are required when oauth is enabled")
		}
		if cfg.OAuth.JWKSURI == "" {
			return fmt.Errorf("oauth.jwks_uri is required when oauth is enabled")
		}
	}

	// Setup logging
	if cfg.LogPath == "stdin" || cfg.LogPath == "stdout" {
		return fmt.Errorf("misconfiguration: cannot log to %s; please specify 'stderr' or a file path", cfg.LogPath)
	}

	var logWriter *os.File
	switch cfg.LogPath {
	case "stderr", "":
		logWriter = os.Stderr
	default:
		f, err := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file %q: %w", cfg.LogPath, err)
		}
		defer f.Close()
		logWriter = f
	}

	logLevel := slog.LevelInfo
	if cfg.Debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(logging.NewTraceHandler(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{Level: logLevel})))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := telemetry.Init(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			slog.Error("telemetry shutdown", "error", err)
		}
	}()

	result, err := parser.ParseMakefiles(cfg.Makefiles, parser.ParseOptions{
		Delimiter: cfg.Delimiter,
		Strict:    cfg.Strict,
	})
	if err != nil {
		return err
	}
	if !result.Valid() {
		return formatValidationError(result.Errors)
	}

	server, err := makecpserver.New(cfg, result.Recipes)
	if err != nil {
		return err
	}

	// Build the OAuth validator when OAuth is enabled.
	var validator *auth.Validator
	if cfg.OAuth.Enabled {
		httpClient, err := auth.NewHTTPClient(cfg.OAuth.TLS)
		if err != nil {
			return fmt.Errorf("auth: build HTTP client: %w", err)
		}
		validator = auth.New(cfg.OAuth.Issuer, cfg.OAuth.Audience, cfg.OAuth.JWKSURI, httpClient)
	}

	// Build the ResourceManager when resources are configured.
	var rm *resources.ResourceManager
	var resourceWatchPaths []string
	if len(cfg.Resources.Paths) > 0 {
		rm, resourceWatchPaths, err = resources.New(cfg.Resources, cfg.OAuth, server.MCP())
		if err != nil {
			return fmt.Errorf("resources: %w", err)
		}

		// Register subscribe/unsubscribe notification handlers.
		server.MCP().AddNotificationHandler("resources/subscribe",
			func(ctx context.Context, notif mcp.JSONRPCNotification) {
				session := mcpserver.ClientSessionFromContext(ctx)
				if session == nil {
					return
				}
				uri, _ := notif.Params.AdditionalFields["uri"].(string)
				rm.Subscribe(session.SessionID(), uri)
			})
		server.MCP().AddNotificationHandler("resources/unsubscribe",
			func(ctx context.Context, notif mcp.JSONRPCNotification) {
				session := mcpserver.ClientSessionFromContext(ctx)
				if session == nil {
					return
				}
				uri, _ := notif.Params.AdditionalFields["uri"].(string)
				rm.Unsubscribe(session.SessionID(), uri)
			})
	}

	// Set up file watcher for Makefiles, config, and resource files.
	watchPaths := append([]string{}, cfg.Makefiles...)
	if configPath != "" {
		watchPaths = append(watchPaths, configPath)
	}
	watchPaths = append(watchPaths, resourceWatchPaths...)

	// Build a set of Makefile/config paths for routing in the watch loop.
	makefileSet := make(map[string]struct{})
	for _, mf := range cfg.Makefiles {
		abs, err := filepath.Abs(mf)
		if err == nil {
			makefileSet[abs] = struct{}{}
		} else {
			makefileSet[mf] = struct{}{}
		}
	}
	if configPath != "" {
		abs, err := filepath.Abs(configPath)
		if err == nil {
			makefileSet[abs] = struct{}{}
		} else {
			makefileSet[configPath] = struct{}{}
		}
	}

	fileWatcher, err := watcher.New(watchPaths, 0)
	if err != nil {
		return err
	}
	defer fileWatcher.Close()

	go routingWatchLoop(ctx, fileWatcher, makefileSet, rm, &cfg, configPath, cliOverride, server)

	errCh := make(chan error, 2)
	if cfg.Transport == "stdio" || cfg.Transport == "both" {
		telemetry.Metrics().ConnectedClients.Add(ctx, 1, metric.WithAttributes(attribute.String("transport", "stdio")))
		go func() {
			defer telemetry.Metrics().ConnectedClients.Add(context.Background(), -1, metric.WithAttributes(attribute.String("transport", "stdio")))
			errCh <- transport.ServeStdio(server.MCP())
		}()
	}

	var httpServer *transport.HTTPServer
	if cfg.Transport == "http" || cfg.Transport == "both" {
		httpServer = transport.NewHTTPServer(server, cfg.Listen,
			transport.WithValidator(validator),
			transport.WithTLS(cfg.OAuth.TLS),
			transport.WithOAuthIssuer(cfg.OAuth.Issuer),
		)
		telemetry.Metrics().ConnectedClients.Add(ctx, 1, metric.WithAttributes(attribute.String("transport", "http")))
		go func() {
			defer telemetry.Metrics().ConnectedClients.Add(context.Background(), -1, metric.WithAttributes(attribute.String("transport", "http")))
			errCh <- httpServer.Start()
		}()
	}

	slog.Info("server started", "transport", cfg.Transport, "makefiles", cfg.Makefiles)

	select {
	case <-ctx.Done():
		if httpServer != nil {
			_ = httpServer.Shutdown(context.Background())
		}
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
}

func loadConfig(configPath string, override config.Config) (config.Config, error) {
	cfg := config.Default()
	if configPath != "" {
		fileCfg, err := config.LoadFile(configPath)
		if err != nil {
			return config.Config{}, fmt.Errorf("loading config: %w", err)
		}
		cfg = config.Merge(cfg, fileCfg)
	}
	return config.Merge(cfg, override), nil
}

// routingWatchLoop routes file-change events from the watcher to either the
// Makefile reload path or the ResourceManager, depending on which set the
// changed path belongs to. A path may match both (e.g. a config file that is
// also a registered resource).
func routingWatchLoop(
	ctx context.Context,
	fileWatcher *watcher.Watcher,
	makefileSet map[string]struct{},
	rm *resources.ResourceManager,
	cfg *config.Config,
	configPath string,
	cliOverride config.Config,
	server *makecpserver.ToolServer,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-fileWatcher.Errors():
			if !ok {
				return
			}
			slog.Error("watch error", "error", err)
		case event, ok := <-fileWatcher.Events():
			if !ok {
				return
			}
			absPath, err := filepath.Abs(event.Path)
			if err != nil {
				absPath = event.Path
			}
			slog.Info("file change detected", "path", absPath)

			_, isMakefile := makefileSet[absPath]
			isResource := rm != nil && rm.IsResourcePath(absPath)

			if isMakefile {
				nextCfg := *cfg
				if configPath != "" && absPath == mustAbs(configPath) {
					reloaded, err := loadConfig(configPath, config.Config{})
					if err != nil {
						slog.Error("reload config failed", "path", absPath, "error", err)
						telemetry.RecordToolReload(ctx, absPath, telemetry.StatusFailure)
						goto handleResource
					}
					nextCfg = config.Merge(reloaded, cliOverride)
					slog.Info("config reloaded", "path", absPath)
				}

				result, err := parser.ParseMakefiles(nextCfg.Makefiles, parser.ParseOptions{
					Delimiter: nextCfg.Delimiter,
					Strict:    nextCfg.Strict,
				})
				if err != nil {
					slog.Error("reload parse failed", "path", absPath, "error", err)
					telemetry.RecordToolReload(ctx, absPath, telemetry.StatusFailure)
					goto handleResource
				}
				if !result.Valid() {
					err := formatValidationError(result.Errors)
					slog.Error("reload validation failed", "path", absPath, "error", err)
					telemetry.RecordToolReload(ctx, absPath, telemetry.StatusFailure)
					goto handleResource
				}
				if err := server.Reload(ctx, result.Recipes, absPath); err != nil {
					slog.Error("server reload failed", "path", absPath, "error", err)
					goto handleResource
				}
				*cfg = nextCfg
				slog.Info("server reloaded successfully", "path", absPath)
			}

		handleResource:
			if isResource {
				rm.HandleFileChange(absPath)
			}
		}
	}
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func formatValidationError(errs []parser.ValidationError) error {
	var parts []string
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return errors.New(strings.Join(parts, "; "))
}
