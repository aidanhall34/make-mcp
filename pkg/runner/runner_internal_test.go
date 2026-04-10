package runner

import (
	"strings"
	"testing"

	"github.com/aidanhall34/make-mcp/internal/testtel"
	"github.com/aidanhall34/make-mcp/pkg/parser"
)

// ---- Error.Error ----

func TestError_Error_Nil(t *testing.T) {
	var e *Error
	if got := e.Error(); got != "" {
		t.Errorf("nil Error.Error() = %q, want empty string", got)
	}
}

func TestError_Error_Message(t *testing.T) {
	e := &Error{Message: "something went wrong"}
	if got := e.Error(); got != "something went wrong" {
		t.Errorf("Error.Error() = %q, want %q", got, "something went wrong")
	}
}

// ---- stringifyValue ----

func TestStringifyValue_AllIntegerVariants(t *testing.T) {
	param := parser.Param{Name: "n", Type: parser.ParamTypeInt}
	cases := []struct {
		input any
		want  string
	}{
		{int(42), "42"},
		{int8(8), "8"},
		{int16(16), "16"},
		{int32(32), "32"},
		{int64(64), "64"},
		{uint(10), "10"},
		{uint8(20), "20"},
		{uint16(30), "30"},
		{uint32(40), "40"},
		{uint64(50), "50"},
		{float64(100), "100"},
		{float32(7), "7"},
	}
	for _, tc := range cases {
		got, err := stringifyValue(param, tc.input)
		if err != nil {
			t.Errorf("stringifyValue(%T(%v)): unexpected error: %v", tc.input, tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("stringifyValue(%T(%v)) = %q, want %q", tc.input, tc.input, got, tc.want)
		}
	}
}

func TestStringifyValue_Float64NonInteger(t *testing.T) {
	param := parser.Param{Name: "n", Type: parser.ParamTypeInt}
	if _, err := stringifyValue(param, float64(1.5)); err == nil {
		t.Error("expected error for non-integer float64, got nil")
	}
}

func TestStringifyValue_Float32NonInteger(t *testing.T) {
	param := parser.Param{Name: "n", Type: parser.ParamTypeInt}
	if _, err := stringifyValue(param, float32(2.5)); err == nil {
		t.Error("expected error for non-integer float32, got nil")
	}
}

func TestStringifyValue_WrongTypeForString(t *testing.T) {
	param := parser.Param{Name: "s", Type: parser.ParamTypeString}
	if _, err := stringifyValue(param, 42); err == nil {
		t.Error("expected error for non-string value on string param, got nil")
	}
}

func TestStringifyValue_WrongTypeForBool(t *testing.T) {
	param := parser.Param{Name: "b", Type: parser.ParamTypeBool}
	if _, err := stringifyValue(param, "true"); err == nil {
		t.Error("expected error for non-bool value on bool param, got nil")
	}
}

func TestStringifyValue_WrongTypeForInt(t *testing.T) {
	param := parser.Param{Name: "n", Type: parser.ParamTypeInt}
	if _, err := stringifyValue(param, "42"); err == nil {
		t.Error("expected error for non-numeric string on int param, got nil")
	}
}

func TestStringifyValue_UnknownParamType(t *testing.T) {
	param := parser.Param{Name: "x", Type: "float"}
	if _, err := stringifyValue(param, 1.0); err == nil {
		t.Error("expected error for unknown param type, got nil")
	}
}

// ---- buildArgs ----

func TestBuildArgs_ParamNotProvided(t *testing.T) {
	recipe := parser.Recipe{
		ID:     "hello",
		Params: []parser.Param{{Name: "name", Type: parser.ParamTypeString}},
	}
	// Provide no values — the param should be silently skipped.
	args, err := buildArgs(recipe, nil)
	if err != nil {
		t.Fatalf("buildArgs() error = %v", err)
	}
	for _, a := range args {
		if strings.Contains(a, "NAME") {
			t.Errorf("unexpected NAME in args: %v", args)
		}
	}
}

func TestBuildArgs_StringifyError(t *testing.T) {
	recipe := parser.Recipe{
		ID:     "hello",
		Params: []parser.Param{{Name: "count", Type: parser.ParamTypeInt}},
	}
	// Providing a string for an int param causes a stringify error.
	if _, err := buildArgs(recipe, map[string]any{"count": "not-a-number"}); err == nil {
		t.Error("expected error for wrong param type, got nil")
	}
}

func TestBuildArgs_WithSourceFile(t *testing.T) {
	recipe := parser.Recipe{
		ID:         "hello",
		SourceFile: "/tmp/some/path/Makefile",
		Params:     []parser.Param{},
	}
	args, err := buildArgs(recipe, nil)
	if err != nil {
		t.Fatalf("buildArgs() error = %v", err)
	}
	fIdx := -1
	for i, a := range args {
		if a == "-f" {
			fIdx = i
			break
		}
	}
	if fIdx == -1 {
		t.Fatalf("expected -f flag in args for recipe with SourceFile; got %v", args)
	}
	if fIdx+1 >= len(args) || args[fIdx+1] != "Makefile" {
		t.Fatalf("expected -f Makefile, got args[%d+1]=%q", fIdx, args[fIdx+1])
	}
}

func TestBuildArgs_BoolAndIntParams(t *testing.T) {
	recipe := parser.Recipe{
		ID: "run",
		Params: []parser.Param{
			{Name: "verbose", Type: parser.ParamTypeBool},
			{Name: "count", Type: parser.ParamTypeInt},
		},
	}
	args, err := buildArgs(recipe, map[string]any{
		"verbose": true,
		"count":   float64(3),
	})
	if err != nil {
		t.Fatalf("buildArgs() error = %v", err)
	}
	found := map[string]bool{}
	for _, a := range args {
		if a == "VERBOSE=true" || a == "COUNT=3" {
			found[a] = true
		}
	}
	if !found["VERBOSE=true"] {
		t.Errorf("VERBOSE=true not in args: %v", args)
	}
	if !found["COUNT=3"] {
		t.Errorf("COUNT=3 not in args: %v", args)
	}
}

// ---- makeVariableName ----

func TestMakeVariableName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"name", "NAME"},
		{"my-param", "MY_PARAM"},
		{"already_set", "ALREADY_SET"},
		{"x", "X"},
		{"multi-word-param", "MULTI_WORD_PARAM"},
	}
	for _, tc := range cases {
		if got := makeVariableName(tc.in); got != tc.want {
			t.Errorf("makeVariableName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---- Benchmarks ----

var benchArgs []string

func BenchmarkBuildArgs_NoParams(b *testing.B) {
	_ = testtel.Start(b)
	recipe := parser.Recipe{ID: "hello", Params: []parser.Param{}}
	b.ResetTimer()
	for b.Loop() {
		benchArgs, _ = buildArgs(recipe, nil)
	}
}

func BenchmarkBuildArgs_FiveParams(b *testing.B) {
	_ = testtel.Start(b)
	recipe := parser.Recipe{
		ID: "greet",
		Params: []parser.Param{
			{Name: "name", Type: parser.ParamTypeString},
			{Name: "count", Type: parser.ParamTypeInt},
			{Name: "shout", Type: parser.ParamTypeBool},
			{Name: "prefix", Type: parser.ParamTypeString},
			{Name: "suffix", Type: parser.ParamTypeString},
		},
	}
	provided := map[string]any{
		"name":   "Alice",
		"count":  float64(5),
		"shout":  true,
		"prefix": "Hello",
		"suffix": "!",
	}
	b.ResetTimer()
	for b.Loop() {
		benchArgs, _ = buildArgs(recipe, provided)
	}
}

var benchStr string

func BenchmarkStringifyValue_String(b *testing.B) {
	_ = testtel.Start(b)
	param := parser.Param{Name: "s", Type: parser.ParamTypeString}
	b.ResetTimer()
	for b.Loop() {
		benchStr, _ = stringifyValue(param, "hello world")
	}
}

func BenchmarkStringifyValue_Int(b *testing.B) {
	_ = testtel.Start(b)
	param := parser.Param{Name: "n", Type: parser.ParamTypeInt}
	b.ResetTimer()
	for b.Loop() {
		benchStr, _ = stringifyValue(param, float64(42))
	}
}
