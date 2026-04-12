package server_test

import (
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/internal/testtel"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	pkgserver "github.com/aidanhall34/make-mcp/pkg/server"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
)

func TestNewBuildsToolRegistry(t *testing.T) {
	ctx := testtel.Start(t)
	_ = ctx
	s, err := pkgserver.New(config.Default(), []parser.Recipe{
		{
			ID:          "hello-world",
			Name:        "Hello World",
			Description: "Prints a greeting.",
			Risk:        parser.RiskLow,
			Params:      []parser.Param{},
			Output:      "Greeting",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !s.Ready() {
		t.Fatal("Ready() = false, want true")
	}
	if tool := s.MCP().GetTool("hello-world"); tool == nil {
		t.Fatal("GetTool(hello-world) = nil, want tool")
	}
}

func TestReloadReplacesTools(t *testing.T) {
	ctx := testtel.Start(t)
	s, err := pkgserver.New(config.Default(), []parser.Recipe{
		{
			ID:          "one",
			Name:        "One",
			Description: "First tool.",
			Risk:        parser.RiskLow,
			Params:      []parser.Param{},
			Output:      "out",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = s.Reload(ctx, []parser.Recipe{
		{
			ID:          "two",
			Name:        "Two",
			Description: "Second tool.",
			Risk:        parser.RiskLow,
			Params:      []parser.Param{},
			Output:      "out",
			OutputType:  "text/plain",
		},
	}, "test")
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if tool := s.MCP().GetTool("one"); tool != nil {
		t.Fatal("old tool still registered after reload")
	}
	if tool := s.MCP().GetTool("two"); tool == nil {
		t.Fatal("new tool missing after reload")
	}
}

func TestReadyFalseWhenNoToolsRegistered(t *testing.T) {
	ctx := testtel.Start(t)
	_ = ctx
	s, err := pkgserver.New(config.Default(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if s.Ready() {
		t.Fatal("Ready() = true, want false")
	}
}

// TestListToolsLatency measures the client-side latency of the server's
// tools/list operation and records it as a histogram metric. The metric
// includes a "pass"/"fail" status label as required by the test telemetry spec.
func TestListToolsLatency(t *testing.T) {
	ctx := testtel.Start(t)
	s, err := pkgserver.New(config.Default(), []parser.Recipe{
		{
			ID:          "alpha",
			Name:        "Alpha",
			Description: "First tool.",
			Risk:        parser.RiskLow,
			Params:      []parser.Param{},
			Output:      "out",
			OutputType:  "text/plain",
		},
		{
			ID:          "beta",
			Name:        "Beta",
			Description: "Second tool.",
			Risk:        parser.RiskLow,
			Params:      []parser.Param{},
			Output:      "out",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	start := time.Now()
	tools := s.MCP().ListTools()
	elapsed := time.Since(start)

	status := telemetry.StatusSuccess
	if len(tools) == 0 {
		status = telemetry.StatusFailure
	}
	telemetry.RecordToolsListRequest(ctx, status, elapsed)

	if len(tools) != 2 {
		t.Errorf("ListTools() returned %d tools, want 2", len(tools))
	}
}
