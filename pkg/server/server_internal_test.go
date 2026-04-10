package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/internal/testtel"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/aidanhall34/make-mcp/pkg/runner"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---- measureListBytes ----

func TestMeasureListBytes_NoCursor(t *testing.T) {
	in, out := measureListBytes("", []mcp.Tool{})
	if in != 0 {
		t.Errorf("bytesIn = %d, want 0", in)
	}
	if out == 0 {
		t.Error("bytesOut = 0 for empty tools slice, want > 0 (JSON [])")
	}
}

func TestMeasureListBytes_WithCursorAndTools(t *testing.T) {
	cursor := mcp.Cursor("abc123")
	tools := []mcp.Tool{{Name: "greet"}, {Name: "build"}}
	in, out := measureListBytes(cursor, tools)
	if in != int64(len("abc123")) {
		t.Errorf("bytesIn = %d, want %d", in, len("abc123"))
	}
	if out == 0 {
		t.Error("bytesOut = 0 for non-empty tools slice")
	}
}

// ---- timeoutFor ----

func TestTimeoutFor(t *testing.T) {
	s := &ToolServer{
		cfg: config.Config{
			Timeouts: config.Timeouts{
				Low:    1 * time.Minute,
				Medium: 2 * time.Minute,
				High:   3 * time.Minute,
			},
		},
	}
	cases := []struct {
		risk parser.RiskLevel
		want time.Duration
	}{
		{parser.RiskLow, 1 * time.Minute},
		{parser.RiskMedium, 2 * time.Minute},
		{parser.RiskHigh, 3 * time.Minute},
		{"unknown", 1 * time.Minute}, // falls through to default (Low)
	}
	for _, tc := range cases {
		if got := s.timeoutFor(tc.risk); got != tc.want {
			t.Errorf("timeoutFor(%q) = %v, want %v", tc.risk, got, tc.want)
		}
	}
}

// ---- handler helpers ----

func TestRunnerErrorToJSONRPC_WithExitCode(t *testing.T) {
	code := 2
	err := runnerErrorToJSONRPC(&runner.Error{
		Code:     runner.ErrorCodeExitNonZero,
		Message:  "make target failed with exit code 2",
		ExitCode: &code,
		Stdout:   "partial out",
		Stderr:   "partial err",
	})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestRunnerErrorToJSONRPC_Timeout(t *testing.T) {
	err := runnerErrorToJSONRPC(&runner.Error{
		Code:     runner.ErrorCodeTimeout,
		Message:  "timed out after 10m",
		TimedOut: true,
	})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestJsonRPCError_NilData(t *testing.T) {
	err := jsonRPCError(-32601, "method not found", nil)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestJsonRPCError_WithData(t *testing.T) {
	err := jsonRPCError(-32000, "internal error", map[string]any{"detail": "boom"})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

// ---- toolMetadata ----

func TestToolMetadata_WithSourceFile(t *testing.T) {
	meta := toolMetadata(parser.Recipe{
		ID:         "hello",
		Name:       "Hello",
		Risk:       parser.RiskLow,
		Output:     "greeting",
		OutputType: "text/plain",
		SourceFile: "/path/to/Makefile",
	})
	if meta["make-mcp.recipe.source_file"] != "/path/to/Makefile" {
		t.Errorf("source_file = %v, want /path/to/Makefile", meta["make-mcp.recipe.source_file"])
	}
}

func TestToolMetadata_WithoutSourceFile(t *testing.T) {
	meta := toolMetadata(parser.Recipe{ID: "hello", Name: "Hello"})
	if _, ok := meta["make-mcp.recipe.source_file"]; ok {
		t.Error("source_file key should be absent when SourceFile is empty")
	}
}

// ---- schemaType ----

func TestSchemaType(t *testing.T) {
	cases := []struct {
		input parser.ParamType
		want  string
	}{
		{parser.ParamTypeInt, "integer"},
		{parser.ParamTypeBool, "boolean"},
		{parser.ParamTypeString, "string"},
		{"unknown", "string"},
	}
	for _, tc := range cases {
		if got := schemaType(tc.input); got != tc.want {
			t.Errorf("schemaType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---- handleToolCall ----

func TestHandleToolCall_UnknownTool(t *testing.T) {
	ctx := testtel.Start(t)
	s, err := New(config.Default(), []parser.Recipe{
		{
			ID:          "hello",
			Name:        "Hello",
			Description: "Greets.",
			Risk:        parser.RiskLow,
			Params:      []parser.Param{},
			Output:      "greeting",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var req mcp.CallToolRequest
	req.Params.Name = "nonexistent"
	result, err := s.handleToolCall(ctx, req)
	if result != nil {
		t.Error("expected nil result for unknown tool")
	}
	if err == nil {
		t.Error("expected error for unknown tool, got nil")
	}
}

func TestHandleToolCall_Success(t *testing.T) {
	ctx := testtel.Start(t)
	dir := t.TempDir()
	mfPath := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(mfPath, []byte("greet:\n\t@printf 'hi\\n'\n"), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}

	s, err := New(config.Default(), []parser.Recipe{
		{
			ID:          "greet",
			Name:        "Greet",
			Description: "Greets.",
			Risk:        parser.RiskLow,
			SourceFile:  mfPath,
			Params:      []parser.Param{},
			Output:      "greeting",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var req mcp.CallToolRequest
	req.Params.Name = "greet"
	result, err := s.handleToolCall(ctx, req)
	if err != nil {
		t.Fatalf("handleToolCall() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleToolCall_NonRunnerError(t *testing.T) {
	ctx := testtel.Start(t)
	// A recipe whose SourceFile points to a non-existent directory causes
	// exec to fail at the OS level (not a runner.Error).
	s, err := New(config.Default(), []parser.Recipe{
		{
			ID:          "fail",
			Name:        "Fail",
			Description: "Fails at exec level.",
			Risk:        parser.RiskLow,
			SourceFile:  "/nonexistent/dir/that/cannot/exist/Makefile",
			Params:      []parser.Param{},
			Output:      "nothing",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var req mcp.CallToolRequest
	req.Params.Name = "fail"
	result, err := s.handleToolCall(ctx, req)
	if result != nil {
		t.Error("expected nil result")
	}
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestBuildServerTools_WithParams(t *testing.T) {
	recipes := []parser.Recipe{{
		ID:          "greet",
		Name:        "Greet",
		Description: "Greets.",
		Risk:        parser.RiskLow,
		Params: []parser.Param{
			{Name: "name", Type: parser.ParamTypeString, Description: "User name"},
			{Name: "count", Type: parser.ParamTypeInt, Description: "Times to greet"},
		},
		Output:     "greeting",
		OutputType: "text/plain",
	}}
	tools, index, err := buildServerTools(recipes, nil)
	if err != nil {
		t.Fatalf("buildServerTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if _, ok := index["greet"]; !ok {
		t.Error("expected 'greet' in recipe index")
	}
}

func TestHandleToolCall_RunnerError(t *testing.T) {
	ctx := testtel.Start(t)
	dir := t.TempDir()
	mfPath := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(mfPath, []byte("fail:\n\t@exit 2\n"), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}

	s, err := New(config.Default(), []parser.Recipe{
		{
			ID:          "fail",
			Name:        "Fail",
			Description: "Always fails.",
			Risk:        parser.RiskLow,
			SourceFile:  mfPath,
			Params:      []parser.Param{},
			Output:      "nothing",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var req mcp.CallToolRequest
	req.Params.Name = "fail"
	result, err := s.handleToolCall(ctx, req)
	if result != nil {
		t.Error("expected nil result on runner error")
	}
	if err == nil {
		t.Error("expected error from runner, got nil")
	}
}

// ---- Benchmarks ----

var benchTools []interface{}

func BenchmarkBuildServerTools_Small(b *testing.B) {
	_ = testtel.Start(b)
	recipes := benchRecipes(5)
	b.ResetTimer()
	for b.Loop() {
		buildServerTools(recipes, nil) //nolint:errcheck
	}
}

func BenchmarkBuildServerTools_Large(b *testing.B) {
	_ = testtel.Start(b)
	recipes := benchRecipes(50)
	b.ResetTimer()
	for b.Loop() {
		buildServerTools(recipes, nil) //nolint:errcheck
	}
}

func benchRecipes(n int) []parser.Recipe {
	recipes := make([]parser.Recipe, n)
	for i := range recipes {
		id := fmt.Sprintf("tool-%d", i)
		recipes[i] = parser.Recipe{
			ID:          id,
			Name:        "Tool " + id,
			Description: "Does something.",
			Risk:        parser.RiskLow,
			Params: []parser.Param{
				{Name: "input", Type: parser.ParamTypeString, Description: "Input value"},
			},
			Output:     "output",
			OutputType: "text/plain",
		}
	}
	return recipes
}
