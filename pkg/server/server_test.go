package server_test

import (
	"context"
	"testing"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	pkgserver "github.com/aidanhall34/make-mcp/pkg/server"
)

func TestNewBuildsToolRegistry(t *testing.T) {
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

	err = s.Reload(context.Background(), []parser.Recipe{
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
	s, err := pkgserver.New(config.Default(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if s.Ready() {
		t.Fatal("Ready() = true, want false")
	}
}
