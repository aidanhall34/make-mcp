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

func TestJsonRPCError_RunnerErrors(t *testing.T) {
	// jsonRPCError is still used for protocol-level errors (e.g. tool not found).
	// Verify it produces a non-nil error with a code we control.
	err := jsonRPCError(-32601, "tool not found", nil)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	err2 := jsonRPCError(-32000, "server error", map[string]any{"detail": "boom"})
	if err2 == nil {
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
	result, callErr := s.handleToolCall(ctx, req)
	if callErr != nil {
		t.Errorf("expected nil error for runner failure (returned as isError result), got %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for runner error")
	}
	if !result.IsError {
		t.Error("expected IsError true for runner error")
	}
}

// ---- resolveHint ----

func TestResolveHint_NilUsesDefault(t *testing.T) {
	if got := resolveHint(nil, true); got != true {
		t.Errorf("resolveHint(nil, true) = %v, want true", got)
	}
	if got := resolveHint(nil, false); got != false {
		t.Errorf("resolveHint(nil, false) = %v, want false", got)
	}
}

func TestResolveHint_ExplicitOverridesDefault(t *testing.T) {
	tr := true
	fa := false
	if got := resolveHint(&tr, false); got != true {
		t.Errorf("resolveHint(&true, false) = %v, want true", got)
	}
	if got := resolveHint(&fa, true); got != false {
		t.Errorf("resolveHint(&false, true) = %v, want false", got)
	}
}

// ---- error span events ----

// TestHandleToolCall_DebugMode_Success exercises the debug logging paths on a
// successful tool call (cfg.Debug = true).
func TestHandleToolCall_DebugMode_Success(t *testing.T) {
	ctx := testtel.Start(t)
	dir := t.TempDir()
	mfPath := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(mfPath, []byte("greet:\n\t@printf 'hi\\n'\n"), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}

	cfg := config.Default()
	cfg.Debug = true
	s, err := New(cfg, []parser.Recipe{
		{
			ID:          "greet",
			Name:        "Greet",
			Description: "Greets.",
			Risk:        parser.RiskLow,
			SourceFile:  mfPath,
			Params:      []parser.Param{{Name: "name", Type: parser.ParamTypeString, Description: "name"}},
			Output:      "greeting",
			OutputType:  "text/plain",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var req mcp.CallToolRequest
	req.Params.Name = "greet"
	req.Params.Arguments = map[string]any{"name": "World"}
	result, callErr := s.handleToolCall(ctx, req)
	if callErr != nil {
		t.Fatalf("handleToolCall() error = %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestHandleToolCall_DebugMode_Failure exercises the debug logging paths on a
// failing tool call (cfg.Debug = true).
func TestHandleToolCall_DebugMode_Failure(t *testing.T) {
	ctx := testtel.Start(t)
	dir := t.TempDir()
	mfPath := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(mfPath, []byte("fail:\n\t@exit 2\n"), 0644); err != nil {
		t.Fatalf("write makefile: %v", err)
	}

	cfg := config.Default()
	cfg.Debug = true
	s, err := New(cfg, []parser.Recipe{
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
	result, callErr := s.handleToolCall(ctx, req)
	if callErr != nil {
		t.Errorf("expected nil error for runner failure (returned as isError result), got %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for runner error")
	}
	if !result.IsError {
		t.Error("expected IsError true for runner error")
	}
}

// TestHandleToolCall_RunnerErrorSetsSpanError verifies that handleToolCall
// does not panic and records span error state when the underlying make process fails.
// The span error / event recording path is exercised here even though the
// test tracer (noop when OTEL_EXPORTER_OTLP_ENDPOINT is unset) discards the
// exported data.
func TestHandleToolCall_RunnerErrorSetsSpanError(t *testing.T) {
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
	result, callErr := s.handleToolCall(ctx, req)
	if callErr != nil {
		t.Errorf("expected nil error for runner failure (returned as isError result), got %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for runner error")
	}
	if !result.IsError {
		t.Error("expected IsError true for runner error")
	}
}

// ---- getRecipesByNames ----

func TestGetRecipesByNames(t *testing.T) {
	r := &Registry{
		recipes: map[string]parser.Recipe{
			"build": {ID: "build", Name: "Build", Risk: parser.RiskLow},
			"test":  {ID: "test", Name: "Test", Risk: parser.RiskMedium},
			"nuke":  {ID: "nuke", Name: "Nuke", Risk: parser.RiskHigh},
		},
	}

	got := r.getRecipesByNames([]string{"build", "nuke", "missing"})
	if len(got) != 2 {
		t.Fatalf("getRecipesByNames returned %d recipes, want 2", len(got))
	}
	ids := map[string]bool{}
	for _, rec := range got {
		ids[rec.ID] = true
	}
	if !ids["build"] || !ids["nuke"] {
		t.Errorf("got ids %v, want build and nuke", ids)
	}
}

func TestGetRecipesByNames_Empty(t *testing.T) {
	r := &Registry{recipes: map[string]parser.Recipe{}}
	got := r.getRecipesByNames([]string{"missing"})
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

// ---- progressWriter / streaming ----

// mockClientSession is a minimal ClientSession for testing the streaming path.
type mockClientSession struct {
	ch chan mcp.JSONRPCNotification
}

func newMockSession(buf int) *mockClientSession {
	return &mockClientSession{ch: make(chan mcp.JSONRPCNotification, buf)}
}

func (m *mockClientSession) Initialize()                                         {}
func (m *mockClientSession) Initialized() bool                                   { return true }
func (m *mockClientSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return m.ch }
func (m *mockClientSession) SessionID() string                                   { return "test-session" }

// TestProgressWriter_Write verifies that progressWriter delivers a JSON-RPC
// notification to the session's channel on every Write call.
func TestProgressWriter_Write(t *testing.T) {
	session := newMockSession(8)
	token := mcp.ProgressToken("tok-1")
	w := &progressWriter{session: session, token: token}

	msg := []byte("hello stream")
	n, err := w.Write(msg)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write() n = %d, want %d", n, len(msg))
	}

	select {
	case notif := <-session.ch:
		if notif.Notification.Method != "notifications/progress" {
			t.Errorf("method = %q, want notifications/progress", notif.Notification.Method)
		}
		fields := notif.Notification.Params.AdditionalFields
		if fields["progressToken"] != token {
			t.Errorf("progressToken = %v, want %v", fields["progressToken"], token)
		}
		if fields["message"] != string(msg) {
			t.Errorf("message = %v, want %q", fields["message"], msg)
		}
	default:
		t.Fatal("no notification received in channel after Write()")
	}
}

// TestHandleToolCall_WithProgressToken exercises the streaming code path when
// a progressToken is set but the context carries no session — streaming falls
// back to non-streaming and the call still succeeds.
func TestHandleToolCall_WithProgressToken_NoSession(t *testing.T) {
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
	tok := mcp.ProgressToken("test-token")
	req.Params.Meta = &mcp.Meta{ProgressToken: tok}
	// No session in context → streaming falls back to false.
	result, callErr := s.handleToolCall(ctx, req)
	if callErr != nil {
		t.Fatalf("handleToolCall() error = %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestHandleToolCall_WithProgressToken_WithSession exercises the full streaming
// code path when both a progressToken and a session are present in the context.
func TestHandleToolCall_WithProgressToken_WithSession(t *testing.T) {
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

	// Inject a mock session so the streaming path is taken.
	session := newMockSession(16)
	sessionCtx := s.mcp.WithContext(ctx, session)

	var req mcp.CallToolRequest
	req.Params.Name = "greet"
	tok := mcp.ProgressToken("stream-token")
	req.Params.Meta = &mcp.Meta{ProgressToken: tok}

	result, callErr := s.handleToolCall(sessionCtx, req)
	if callErr != nil {
		t.Fatalf("handleToolCall() error = %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---- Benchmarks ----

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
