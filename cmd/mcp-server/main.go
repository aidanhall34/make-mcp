package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	makecpserver "github.com/aidanhall34/make-mcp/pkg/server"
	"github.com/aidanhall34/make-mcp/pkg/server/transport"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
	"github.com/aidanhall34/make-mcp/pkg/watcher"
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
		strict     bool
	)

	fs.StringVar(&configPath, "config", "", "path to make-mcp.yml configuration file")
	fs.StringVar(&delimiter, "delimiter", "", "annotation delimiter")
	fs.Var(&makefiles, "makefile", "Makefile path to parse; may be repeated")
	fs.StringVar(&transportf, "transport", "", "transport to enable: stdio, http, or both")
	fs.StringVar(&listen, "listen", "", "HTTP listen address")
	fs.BoolVar(&strict, "strict", false, "require every supported recipe annotation, including optional MCP tool hints")

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
		Strict:    strict,
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := telemetry.Init(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Printf("telemetry shutdown: %v", err)
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

	watchPaths := append([]string{}, cfg.Makefiles...)
	if configPath != "" {
		watchPaths = append(watchPaths, configPath)
	}

	fileWatcher, err := watcher.New(watchPaths, 0)
	if err != nil {
		return err
	}
	defer fileWatcher.Close()

	go watchLoop(ctx, fileWatcher, &cfg, configPath, cliOverride, server)

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
		httpServer = transport.NewHTTPServer(server, cfg.Listen)
		telemetry.Metrics().ConnectedClients.Add(ctx, 1, metric.WithAttributes(attribute.String("transport", "http")))
		go func() {
			defer telemetry.Metrics().ConnectedClients.Add(context.Background(), -1, metric.WithAttributes(attribute.String("transport", "http")))
			errCh <- httpServer.Start()
		}()
	}

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

func watchLoop(ctx context.Context, fileWatcher *watcher.Watcher, cfg *config.Config, configPath string, cliOverride config.Config, server *makecpserver.ToolServer) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-fileWatcher.Errors():
			if !ok {
				return
			}
			log.Printf("watch error: %v", err)
		case event, ok := <-fileWatcher.Events():
			if !ok {
				return
			}
			nextCfg := *cfg
			if configPath != "" && event.Path == configPath {
				reloaded, err := loadConfig(configPath, config.Config{})
				if err != nil {
					log.Printf("reload config: %v", err)
					telemetry.RecordToolReload(ctx, event.Path, telemetry.StatusFailure)
					continue
				}
				nextCfg = config.Merge(reloaded, cliOverride)
			}

			result, err := parser.ParseMakefiles(nextCfg.Makefiles, parser.ParseOptions{
				Delimiter: nextCfg.Delimiter,
				Strict:    nextCfg.Strict,
			})
			if err != nil {
				log.Printf("reload parse: %v", err)
				telemetry.RecordToolReload(ctx, event.Path, telemetry.StatusFailure)
				continue
			}
			if !result.Valid() {
				log.Printf("reload failed: %v", formatValidationError(result.Errors))
				telemetry.RecordToolReload(ctx, event.Path, telemetry.StatusFailure)
				continue
			}
			if err := server.Reload(ctx, result.Recipes, event.Path); err != nil {
				log.Printf("reload failed: %v", err)
				continue
			}
			*cfg = nextCfg
		}
	}
}

func formatValidationError(errs []parser.ValidationError) error {
	var parts []string
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return errors.New(strings.Join(parts, "; "))
}
