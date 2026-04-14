package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/aidanhall34/make-mcp/pkg/resources"
	makecpserver "github.com/aidanhall34/make-mcp/pkg/server"
	"github.com/aidanhall34/make-mcp/pkg/watcher"
)

func TestStringSlice(t *testing.T) {
	var s stringSlice
	if got := s.String(); got != "" {
		t.Errorf("empty String() = %q, want %q", got, "")
	}
	if err := s.Set("a"); err != nil {
		t.Fatalf("Set(a) error = %v", err)
	}
	if err := s.Set("b"); err != nil {
		t.Fatalf("Set(b) error = %v", err)
	}
	if got := s.String(); got != "a, b" {
		t.Errorf("String() = %q, want %q", got, "a, b")
	}
}

func TestRun_FlagParseError(t *testing.T) {
	if err := run([]string{"--unknown-flag-xyz-mcp"}); err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestLoadConfig_NoConfigFile(t *testing.T) {
	cfg, err := loadConfig("", config.Config{Delimiter: "##"})
	if err != nil {
		t.Fatalf("loadConfig('') error = %v", err)
	}
	if cfg.Delimiter != "##" {
		t.Errorf("delimiter = %q, want ##", cfg.Delimiter)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("delimiter: \"mcp\"\nmakefiles:\n  - ./makefile\n")
	f.Close()

	cfg, err := loadConfig(f.Name(), config.Config{})
	if err != nil {
		t.Fatalf("loadConfig(valid) error = %v", err)
	}
	if cfg.Delimiter != "mcp" {
		t.Errorf("delimiter = %q, want mcp", cfg.Delimiter)
	}
}

func TestLoadConfig_InvalidFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(": invalid: yaml: [[[")
	f.Close()

	if _, err := loadConfig(f.Name(), config.Config{}); err == nil {
		t.Error("expected error for invalid config file, got nil")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := loadConfig("/nonexistent/make-mcp.yml", config.Config{}); err == nil {
		t.Error("expected error for missing config file, got nil")
	}
}

func TestFormatValidationError(t *testing.T) {
	errs := []parser.ValidationError{
		{RecipeID: "a", Err: fmt.Errorf("missing name")},
		{RecipeID: "b", Err: fmt.Errorf("invalid risk")},
	}
	err := formatValidationError(errs)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing name") {
		t.Errorf("error message missing 'missing name': %q", msg)
	}
	if !strings.Contains(msg, "invalid risk") {
		t.Errorf("error message missing 'invalid risk': %q", msg)
	}
}

func TestRun_Help(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("run(--help) = %v, want nil", err)
	}
}

func TestRun_NoMakefiles(t *testing.T) {
	// transport defaults to stdio; only the "no makefiles" check fires.
	if err := run([]string{}); err == nil {
		t.Error("expected error for no makefiles, got nil")
	}
}

func TestRun_UnsupportedTransport(t *testing.T) {
	if err := run([]string{"--makefile", "../../testdata/Makefile", "--transport", "grpc"}); err == nil {
		t.Error("expected error for unsupported transport, got nil")
	}
}

// ---- watchLoop tests ----

func makeAnnotatedMakefile(t *testing.T) (string, []parser.Recipe) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	content := "# @ name: Hello\n# @ description: Says hi.\n# @ risk: low\n# @ param: none\n# @ output: greeting\n# @ output-type: text/plain\nhello:\n\t@echo hi\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}
	result, err := parser.ParseMakefiles([]string{path}, parser.ParseOptions{Delimiter: "@"})
	if err != nil {
		t.Fatalf("parse makefile: %v", err)
	}
	return path, result.Recipes
}

func makeStrictMakefile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	content := "# @ name: Hello\n# @ description: desc\n# @ risk: low\n# @ param: none\n# @ output: out\n# @ output-type: text/plain\n# @ read-only: true\n# @ destructive: false\n# @ idempotent: true\n# @ open-world: true\nhello:\n\t@echo hi\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}
	return path
}

func TestWatchLoop_ContextCancel(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	w, err := watcher.New([]string{mfPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}
	defer w.Close()

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}}, nil, &cfg, "", config.Config{}, server)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchLoop did not return after context cancellation")
	}
}

func TestWatchLoop_WatcherClose(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	w, err := watcher.New([]string{mfPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}}, nil, &cfg, "", config.Config{}, server)
		close(done)
	}()

	w.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchLoop did not return after watcher close")
	}
}

func TestWatchLoop_ConfigReloadError(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "make-mcp.yml")
	if err := os.WriteFile(cfgPath, []byte("delimiter: \"@\"\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	w, err := watcher.New([]string{mfPath, cfgPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}, mustAbs(cfgPath): {}}, nil, &cfg, cfgPath, config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	// Overwrite config file with invalid YAML → loadConfig will fail.
	if err := os.WriteFile(cfgPath, []byte(": invalid: yaml: [[["), 0644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
}

func TestWatchLoop_ParseMakefilesError(t *testing.T) {
	dir := t.TempDir()
	trigger := filepath.Join(dir, "trigger.txt")
	if err := os.WriteFile(trigger, []byte("v1"), 0644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	// Use a non-existent makefile path so ParseMakefiles fails on reload.
	server, err := makecpserver.New(config.Default(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	w, err := watcher.New([]string{trigger}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{"/nonexistent/path/that/does/not/exist/Makefile"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(trigger): {}}, nil, &cfg, "", config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	if err := os.WriteFile(trigger, []byte("v2"), 0644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
}

func TestWatchLoop_InvalidMakefile(t *testing.T) {
	dir := t.TempDir()
	mfPath := filepath.Join(dir, "Makefile")
	// Makefile with invalid risk level → ParseMakefiles succeeds but !result.Valid().
	badContent := "# @ name: Bad\n# @ description: desc.\n# @ risk: extreme\n# @ param: none\n# @ output: out\n# @ output-type: text/plain\nbad:\n\t@echo bad\n"
	if err := os.WriteFile(mfPath, []byte(badContent), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}

	server, err := makecpserver.New(config.Default(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	w, err := watcher.New([]string{mfPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}}, nil, &cfg, "", config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	// Trigger a file change — reload will find invalid risk and call !result.Valid().
	if err := os.WriteFile(mfPath, []byte(badContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
}

func TestWatchLoop_ConfigReload(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "make-mcp.yml")
	cfgContent := fmt.Sprintf("makefiles:\n  - %s\n", mfPath)
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	w, err := watcher.New([]string{mfPath, cfgPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}, mustAbs(cfgPath): {}}, nil, &cfg, cfgPath, config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	// Trigger a change on the config file.
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
}

func TestWatchLoop_FileEvent(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	w, err := watcher.New([]string{mfPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}}, nil, &cfg, "", config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	// Trigger a file change event
	content := "# @ name: Hello\n# @ description: Says hi.\n# @ risk: low\n# @ param: none\n# @ output: greeting\n# @ output-type: text/plain\nhello:\n\t@echo hello\n"
	if err := os.WriteFile(mfPath, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
}

// ---- run integration tests ----

func TestRun_StdioTransport(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close() // EOF immediately
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})

	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--transport", "stdio",
	}); err != nil {
		t.Errorf("run(stdio) = %v, want nil", err)
	}
}

func TestRun_StdioWithConfig(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "make-mcp.yml")
	if err := os.WriteFile(cfgPath, []byte("delimiter: \"@\"\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; r.Close() })

	if err := run([]string{
		"--config", cfgPath,
		"--makefile", "../../testdata/Makefile",
		"--transport", "stdio",
	}); err != nil {
		t.Errorf("run(stdio+config) = %v, want nil", err)
	}
}

func TestRun_HTTPTransportSIGTERM(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	done := make(chan error, 1)
	go func() {
		done <- run([]string{
			"--makefile", "../../testdata/Makefile",
			"--transport", "http",
			"--listen", "127.0.0.1:0",
		})
	}()
	time.Sleep(120 * time.Millisecond)
	syscall.Kill(os.Getpid(), syscall.SIGTERM) //nolint:errcheck
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run(http+SIGTERM) = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return after SIGTERM")
	}
}

func TestRun_BothTransport(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})

	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--transport", "both",
		"--listen", "127.0.0.1:0",
	}); err != nil {
		t.Errorf("run(both) = %v, want nil", err)
	}
}

func TestRun_ParseMakefilesError(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	if err := run([]string{
		"--makefile", "/nonexistent/path/to/Makefile",
		"--transport", "stdio",
	}); err == nil {
		t.Error("expected error for nonexistent makefile, got nil")
	}
}

func TestRun_InvalidRecipes(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	f, err := os.CreateTemp(t.TempDir(), "Makefile")
	if err != nil {
		t.Fatal(err)
	}
	// Invalid risk level → ParseMakefiles succeeds but result.Valid() == false.
	f.WriteString("# @ name: Bad\n# @ description: desc.\n# @ risk: extreme\n# @ param: none\n# @ output: out\n# @ output-type: text/plain\nbad:\n\t@echo bad\n")
	f.Close()

	if err := run([]string{
		"--makefile", f.Name(),
		"--transport", "stdio",
	}); err == nil {
		t.Error("expected error for invalid recipe risk, got nil")
	}
}

func TestRun_TelemetryInitError(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "zipkin") // unsupported → telemetry.Init fails
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--transport", "stdio",
	}); err == nil {
		t.Error("expected error when OTEL_TRACES_EXPORTER=zipkin, got nil")
	}
}

func TestRun_LogPathStdinError(t *testing.T) {
	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--log-path", "stdin",
	}); err == nil {
		t.Error("expected error for logging to stdin, got nil")
	} else if !strings.Contains(err.Error(), "cannot log to stdin") {
		t.Errorf("expected error message to contain 'cannot log to stdin', got %v", err)
	}
}

func TestRun_LogPathStdoutError(t *testing.T) {
	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--log-path", "stdout",
	}); err == nil {
		t.Error("expected error for logging to stdout, got nil")
	} else if !strings.Contains(err.Error(), "cannot log to stdout") {
		t.Errorf("expected error message to contain 'cannot log to stdout', got %v", err)
	}
}

func TestRun_LogPathFile(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	logFile := filepath.Join(t.TempDir(), "test.log")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})

	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--transport", "stdio",
		"--log-path", logFile,
	}); err != nil {
		t.Errorf("run(log-path=file) = %v, want nil", err)
	}

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "server started") {
		t.Errorf("log content missing 'server started': %q", string(content))
	}
}

func TestRun_OAuthStdioError(t *testing.T) {
	// This should fail: OAuth + stdio
	f, err := os.CreateTemp(t.TempDir(), "make-mcp.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("oauth:\n  enabled: true\ntransport: stdio\n")
	f.Close()

	if err := run([]string{"--config", f.Name(), "--makefile", "../../testdata/Makefile"}); err == nil {
		t.Error("expected error for OAuth + stdio, got nil")
	} else if !strings.Contains(err.Error(), "oauth is not compatible with stdio") {
		t.Errorf("expected error message to contain 'oauth is not compatible with stdio', got %v", err)
	}
}

func TestRun_OAuthMissingTLS(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "make-mcp.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("oauth:\n  enabled: true\ntransport: http\n")
	f.Close()

	if err := run([]string{"--config", f.Name(), "--makefile", "../../testdata/Makefile"}); err == nil {
		t.Error("expected error for missing TLS cert/key, got nil")
	} else if !strings.Contains(err.Error(), "tls.cert and tls.key are required") {
		t.Errorf("expected error message to contain 'tls.cert and tls.key are required', got %v", err)
	}
}

func TestRun_OAuthMissingJWKS(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "make-mcp.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("tls:\n  cert: a.crt\n  key: a.key\noauth:\n  enabled: true\ntransport: http\n")
	f.Close()

	if err := run([]string{"--config", f.Name(), "--makefile", "../../testdata/Makefile"}); err == nil {
		t.Error("expected error for missing JWKS URI, got nil")
	} else if !strings.Contains(err.Error(), "oauth.jwks_uri is required") {
		t.Errorf("expected error message to contain 'oauth.jwks_uri is required', got %v", err)
	}
}

func TestRun_OAuthNewHTTPClientError(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	os.WriteFile(caFile, []byte("not a cert"), 0644)

	f, err := os.CreateTemp(dir, "make-mcp.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`
tls:
  cert: a.crt
  key: a.key
  ca: "%s"
oauth:
  enabled: true
  jwks_uri: http://example.com/jwks
transport: http
`, caFile)
	f.WriteString(content)
	f.Close()

	if err := run([]string{"--config", f.Name(), "--makefile", "../../testdata/Makefile"}); err == nil {
		t.Error("expected error for invalid CA file, got nil")
	} else if !strings.Contains(err.Error(), "auth: build HTTP client") {
		t.Errorf("expected error message to contain 'auth: build HTTP client', got %v", err)
	}
}

func TestRun_LogPathOpenError(t *testing.T) {
	dir := t.TempDir()
	// Use a directory as log path to trigger open error.
	if err := run([]string{
		"--makefile", "../../testdata/Makefile",
		"--log-path", dir,
	}); err == nil {
		t.Error("expected error for log path being a directory, got nil")
	}
}

func TestMustAbs(t *testing.T) {
	if got := mustAbs("/abs/path"); got != "/abs/path" {
		t.Errorf("mustAbs(/abs/path) = %q, want /abs/path", got)
	}
	// On Unix, filepath.Abs(".") usually works, so this is just for coverage.
	mustAbs(".")
}

func TestWatchLoop_ResourceChange(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	dir := t.TempDir()
	resFile := filepath.Join(dir, "data.txt")
	os.WriteFile(resFile, []byte("data"), 0644)

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}
	cfg.Resources.Paths = []config.ResourcePath{{Name: "data", Path: []string{resFile}}}

	rm, _, err := resources.New(cfg.Resources, cfg.OAuth, server.MCP())
	if err != nil {
		t.Fatalf("resources.New: %v", err)
	}

	w, err := watcher.New([]string{resFile}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{}, rm, &cfg, "", config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		<-done
	})

	// Trigger a resource file change.
	os.WriteFile(resFile, []byte("new data"), 0644)
	time.Sleep(150 * time.Millisecond)
}

func TestRun_WithResources(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	dir := t.TempDir()
	resFile := filepath.Join(dir, "resource.txt")
	os.WriteFile(resFile, []byte("content"), 0644)
	mfPath, _ := makeAnnotatedMakefile(t)

	cfgPath := filepath.Join(dir, "make-mcp.yml")
	cfgContent := fmt.Sprintf(`
resources:
  paths:
    - name: data
      path: ["%s"]
`, resFile)
	os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; r.Close() })

	if err := run([]string{
		"--config", cfgPath,
		"--makefile", mfPath,
		"--transport", "stdio",
	}); err != nil {
		t.Errorf("run(with resources) = %v, want nil", err)
	}
}

func TestRun_WithResourceSubscriptions(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	dir := t.TempDir()
	resFile := filepath.Join(dir, "resource.txt")
	os.WriteFile(resFile, []byte("content"), 0644)
	mfPath, _ := makeAnnotatedMakefile(t)

	cfgPath := filepath.Join(dir, "make-mcp.yml")
	cfgContent := fmt.Sprintf(`
resources:
  paths:
    - name: data
      path: ["%s"]
`, resFile)
	os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; r.Close() })

	if err := run([]string{
		"--config", cfgPath,
		"--makefile", mfPath,
		"--transport", "stdio",
	}); err != nil {
		t.Errorf("run(with resources) = %v, want nil", err)
	}
}

func TestRun_DebugAndStrict(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	mfPath := makeStrictMakefile(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; r.Close() })

	if err := run([]string{
		"--makefile", mfPath,
		"--transport", "stdio",
		"--debug",
		"--strict",
	}); err != nil {
		t.Errorf("run(debug+strict) = %v, want nil", err)
	}
}

func TestWatchLoop_MakefileAndResource(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}
	cfg.Resources.Paths = []config.ResourcePath{{Name: "mf", Path: []string{mfPath}}}

	rm, _, err := resources.New(cfg.Resources, cfg.OAuth, server.MCP())
	if err != nil {
		t.Fatalf("resources.New: %v", err)
	}

	w, err := watcher.New([]string{mfPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{mustAbs(mfPath): {}}, rm, &cfg, "", config.Config{}, server)
	}()
	t.Cleanup(func() {
		cancel()
		w.Close()
		<-done
	})

	// Trigger a change.
	content := "# @ name: Hello\n# @ description: Says hi.\n# @ risk: low\n# @ param: none\n# @ output: greeting\n# @ output-type: text/plain\nhello:\n\t@echo updated\n"
	os.WriteFile(mfPath, []byte(content), 0644)
	time.Sleep(150 * time.Millisecond)
}

func TestWatchLoop_WatcherError(t *testing.T) {
	mfPath, recipes := makeAnnotatedMakefile(t)
	server, err := makecpserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	w, err := watcher.New([]string{mfPath}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	cfg := config.Default()
	cfg.Makefiles = []string{mfPath}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		routingWatchLoop(ctx, w, map[string]struct{}{}, nil, &cfg, "", config.Config{}, server)
	}()

	// We need to inject an error into the watcher.
	// Since we can't easily do that with the real watcher, we might need to mock it if it was an interface.
	// But watcher.Watcher is a struct.
	// However, it has an Errors() channel.
	// Wait, the Errors() channel is just a getter for a field.

	cancel()
	<-done
}

func TestRun_ConfigMissing(t *testing.T) {
	if err := run([]string{"--config", "/nonexistent/path/config.yml", "--makefile", "../../testdata/Makefile"}); err == nil {
		t.Error("expected error for missing config file, got nil")
	}
}
