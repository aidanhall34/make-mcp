package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/internal/testtel"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/aidanhall34/make-mcp/pkg/runner"
)

func TestRunSuccess(t *testing.T) {
	ctx := testtel.Start(t)
	result, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{
			SourceFile: makeTempMakefile(t, `
hello:
	@printf 'hello stdout\n'
	@printf 'hello stderr\n' 1>&2
`),
			ID:     "hello",
			Params: []parser.Param{},
		},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Stdout != "hello stdout\n" {
		t.Fatalf("stdout = %q, want %q", result.Stdout, "hello stdout\n")
	}
	if result.Stderr != "hello stderr\n" {
		t.Fatalf("stderr = %q, want %q", result.Stderr, "hello stderr\n")
	}
}

func TestRunRejectsUnknownParam(t *testing.T) {
	ctx := testtel.Start(t)
	_, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{
			SourceFile: makeTempMakefile(t, "hello:\n\t@true\n"),
			ID:         "hello",
			Params: []parser.Param{
				{Name: "name", Type: parser.ParamTypeString},
			},
		},
		Args: map[string]any{"extra": "nope"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	runnerErr := expectRunnerError(t, err)
	if runnerErr.Code != runner.ErrorCodeInvalidParams {
		t.Fatalf("error code = %q, want %q", runnerErr.Code, runner.ErrorCodeInvalidParams)
	}
}

func TestRunExitNonZero(t *testing.T) {
	ctx := testtel.Start(t)
	_, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{
			SourceFile: makeTempMakefile(t, `
fail:
	@printf 'partial stdout\n'
	@printf 'partial stderr\n' 1>&2
	@exit 2
`),
			ID:     "fail",
			Params: []parser.Param{},
		},
		Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	runnerErr := expectRunnerError(t, err)
	if runnerErr.Code != runner.ErrorCodeExitNonZero {
		t.Fatalf("error code = %q, want %q", runnerErr.Code, runner.ErrorCodeExitNonZero)
	}
	if runnerErr.ExitCode == nil || *runnerErr.ExitCode != 2 {
		t.Fatalf("exit code = %v, want 2", runnerErr.ExitCode)
	}
	if runnerErr.Stdout != "partial stdout\n" {
		t.Fatalf("stdout = %q", runnerErr.Stdout)
	}
	if !strings.Contains(runnerErr.Stderr, "partial stderr\n") {
		t.Fatalf("stderr missing command output: %q", runnerErr.Stderr)
	}
	if !strings.Contains(runnerErr.Stderr, "Error 2") {
		t.Fatalf("stderr missing exit code context: %q", runnerErr.Stderr)
	}
}

func TestRunTimeout(t *testing.T) {
	ctx := testtel.Start(t)
	_, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{
			SourceFile: makeTempMakefile(t, `
slow:
	@sleep 1
`),
			ID:     "slow",
			Params: []parser.Param{},
		},
		Timeout: 10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	runnerErr := expectRunnerError(t, err)
	if runnerErr.Code != runner.ErrorCodeTimeout {
		t.Fatalf("error code = %q, want %q", runnerErr.Code, runner.ErrorCodeTimeout)
	}
	if !runnerErr.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
}

func TestRunCommandInjectionContract(t *testing.T) {
	ctx := testtel.Start(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	makePath := filepath.Join(dir, "make")
	payload := filepath.Join(dir, "pwned")

	script := "#!/bin/sh\nprintf '%s\n' \"$@\" >\"" + logPath + "\"\n"
	if err := os.WriteFile(makePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake make: %v", err)
	}

	_, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{
			ID: "greet",
			Params: []parser.Param{
				{Name: "name", Type: parser.ParamTypeString},
			},
		},
		Args: map[string]any{
			"name": "Alice; touch " + payload + " && echo nope | cat $(whoami) `id`",
		},
		MakePath: makePath,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read arg log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"--no-print-directory",
		"greet",
		"NAME=Alice; touch " + payload + " && echo nope | cat $(whoami) `id`",
	}
	if len(got) != len(want) {
		t.Fatalf("argv len = %d, want %d; argv=%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if _, err := os.Stat(payload); !os.IsNotExist(err) {
		t.Fatalf("payload file state = %v, expected no injected file", err)
	}
}

func expectRunnerError(t *testing.T, err error) *runner.Error {
	t.Helper()
	runnerErr, ok := err.(*runner.Error)
	if !ok {
		t.Fatalf("error type = %T, want *runner.Error", err)
	}
	return runnerErr
}

func TestRunEmptyRecipeID(t *testing.T) {
	ctx := testtel.Start(t)
	_, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{},
	})
	if err == nil {
		t.Fatal("expected error for empty recipe ID, got nil")
	}
	runnerErr := expectRunnerError(t, err)
	if runnerErr.Code != runner.ErrorCodeInvalidParams {
		t.Fatalf("error code = %q, want %q", runnerErr.Code, runner.ErrorCodeInvalidParams)
	}
}

func TestRunDefaultTimeout(t *testing.T) {
	ctx := testtel.Start(t)
	result, err := runner.Run(ctx, runner.Request{
		Recipe: parser.Recipe{
			SourceFile: makeTempMakefile(t, "hello:\n\t@printf 'hi\\n'\n"),
			ID:         "hello",
			Params:     []parser.Param{},
		},
		// Timeout: 0 → uses default 10-minute timeout
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Stdout != "hi\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "hi\n")
	}
}

func TestRunMakeNotFound(t *testing.T) {
	ctx := testtel.Start(t)
	_, err := runner.Run(ctx, runner.Request{
		Recipe:   parser.Recipe{ID: "hello"},
		MakePath: "/nonexistent/make-binary-xyz",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent make binary, got nil")
	}
	// Should NOT be a runner.Error — it's a lower-level exec error.
	if _, ok := err.(*runner.Error); ok {
		t.Fatal("expected non-runner.Error for missing binary, got *runner.Error")
	}
}

func makeTempMakefile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(contents, "\n")), 0o644); err != nil {
		t.Fatalf("write temp makefile: %v", err)
	}
	return path
}
