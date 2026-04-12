package parser_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aidanhall34/make-mcp/pkg/parser"
)

var defaultOpts = parser.ParseOptions{Delimiter: "@"}

// validAnnotations returns a complete, valid annotation block for the given
// target using the default delimiter and no input params.
func validAnnotations(target string) string {
	return `# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
` + target + `:
	@echo "Hello, World!"
`
}

// ----- schema / Recipe.Validate tests -----

func TestRecipeValidate_Valid(t *testing.T) {
	r := parser.Recipe{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "text/plain",
	}
	if errs := r.Validate(); len(errs) != 0 {
		t.Errorf("expected valid recipe, got errors: %v", errs)
	}
}

func TestRecipeValidate_ValidWithParams(t *testing.T) {
	r := parser.Recipe{
		ID:          "greet",
		Name:        "Greet",
		Description: "Greets a user.",
		Risk:        parser.RiskLow,
		Params: []parser.Param{
			{Name: "name", Type: parser.ParamTypeString, Description: "The name to greet"},
		},
		Output:     "A greeting string",
		OutputType: "text/plain",
	}
	if errs := r.Validate(); len(errs) != 0 {
		t.Errorf("expected valid recipe with params, got errors: %v", errs)
	}
}

func assertBoolPtr(t *testing.T, got *bool, want bool, field string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %v", field, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", field, *got, want)
	}
}

func validationErrorContains(errs []parser.ValidationError, text string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), text) {
			return true
		}
	}
	return false
}

func TestRecipeValidate_ValidOutputTypeWithParameters(t *testing.T) {
	r := parser.Recipe{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "application/json; charset=utf-8",
	}
	if errs := r.Validate(); len(errs) != 0 {
		t.Errorf("expected valid recipe, got errors: %v", errs)
	}
}

func TestRecipeValidate_ParamsNil(t *testing.T) {
	// nil Params means no @param annotation was present at all — always an error.
	r := parser.Recipe{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      nil,
		Output:      "A greeting string",
		OutputType:  "text/plain",
	}
	errs := r.Validate()
	if len(errs) == 0 {
		t.Error("expected error for nil Params (missing @param declaration), got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "param") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a param-related error, got: %v", errs)
	}
}

func TestRecipeValidate_ParamsNone(t *testing.T) {
	// An empty non-nil slice means @param: none was declared — valid.
	r := parser.Recipe{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "text/plain",
	}
	if errs := r.Validate(); len(errs) != 0 {
		t.Errorf("expected valid recipe with @param: none, got errors: %v", errs)
	}
}

func TestRecipeValidate_MissingID(t *testing.T) {
	r := parser.Recipe{
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "text/plain",
	}
	if errs := r.Validate(); len(errs) == 0 {
		t.Error("expected errors for missing ID, got none")
	}
}

func TestRecipeValidate_MissingName(t *testing.T) {
	r := parser.Recipe{
		ID:          "hello-world",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "text/plain",
	}
	if errs := r.Validate(); len(errs) == 0 {
		t.Error("expected errors for missing name, got none")
	}
}

func TestRecipeValidate_InvalidRisk(t *testing.T) {
	r := parser.Recipe{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        "extreme",
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "text/plain",
	}
	if errs := r.Validate(); len(errs) == 0 {
		t.Error("expected errors for invalid risk level, got none")
	}
}

func TestRecipeValidate_InvalidOutputType(t *testing.T) {
	r := parser.Recipe{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "Prints a greeting.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A greeting string",
		OutputType:  "not a content type",
	}
	errs := r.Validate()
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid output type, got none")
	}
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "valid HTTP content type") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected output-type validation error, got %v", errs)
	}
}

func TestRecipeValidate_AllRiskLevels(t *testing.T) {
	for _, risk := range []parser.RiskLevel{parser.RiskLow, parser.RiskMedium, parser.RiskHigh} {
		r := parser.Recipe{
			ID:          "target",
			Name:        "Target",
			Description: "desc",
			Risk:        risk,
			Params:      []parser.Param{},
			Output:      "out",
			OutputType:  "text/plain",
		}
		if errs := r.Validate(); len(errs) != 0 {
			t.Errorf("risk %q should be valid, got: %v", risk, errs)
		}
	}
}

func TestRecipeValidate_MultipleErrors(t *testing.T) {
	// A recipe missing several fields should report every violation, not just the first.
	r := parser.Recipe{
		ID:   "target",
		Risk: parser.RiskLow,
		// missing: Name, Description, Params, Output, OutputType
	}
	errs := r.Validate()
	if len(errs) < 2 {
		t.Errorf("expected multiple validation errors, got %d: %v", len(errs), errs)
	}
}

// ----- ParseMakefile — param-specific tests -----

func TestParseMakefile_ParamNone(t *testing.T) {
	// @param: none → Params is an empty non-nil slice, recipe is valid.
	input := `
# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
hello-world:
	@echo "Hello, World!"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	r := result.Recipes[0]
	if r.Params == nil {
		t.Error("Params should be non-nil when @param: none is declared")
	}
	if len(r.Params) != 0 {
		t.Errorf("Params should be empty for @param: none, got %v", r.Params)
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_SingleParam(t *testing.T) {
	input := `
# @ name: Greet User
# @ description: Sends a personalised greeting.
# @ risk: low
# @ param: name string | The name to include in the greeting
# @ output: A greeting string
# @ output-type: text/plain
greet:
	@echo "Hello, $(NAME)!"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	r := result.Recipes[0]
	if len(r.Params) != 1 {
		t.Fatalf("expected 1 param, got %d: %v", len(r.Params), r.Params)
	}
	p := r.Params[0]
	if p.Name != "name" {
		t.Errorf("param Name: got %q, want %q", p.Name, "name")
	}
	if p.Type != parser.ParamTypeString {
		t.Errorf("param Type: got %q, want %q", p.Type, parser.ParamTypeString)
	}
	if p.Description != "The name to include in the greeting" {
		t.Errorf("param Description: got %q", p.Description)
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_ToolHints(t *testing.T) {
	input := `
# @ name: Status
# @ description: Reads local status.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Status text
# @ output-type: text/plain
status:
	@echo "ok"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !result.Valid() {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}

	hints := result.Recipes[0].ToolHints
	assertBoolPtr(t, hints.ReadOnly, true, "ReadOnly")
	assertBoolPtr(t, hints.Destructive, false, "Destructive")
	assertBoolPtr(t, hints.Idempotent, true, "Idempotent")
	assertBoolPtr(t, hints.OpenWorld, false, "OpenWorld")
}

func TestParseMakefile_InvalidToolHint(t *testing.T) {
	input := `
# @ name: Status
# @ description: Reads local status.
# @ risk: low
# @ read-only: maybe
# @ param: none
# @ output: Status text
# @ output-type: text/plain
status:
	@echo "ok"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Fatal("expected validation error for invalid read-only hint")
	}
	found := false
	for _, validationErr := range result.Errors {
		if strings.Contains(validationErr.Error(), "read-only") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected read-only validation error, got: %v", result.Errors)
	}
}

func TestParseMakefile_StrictRequiresToolHints(t *testing.T) {
	input := `
# @ name: Status
# @ description: Reads local status.
# @ risk: low
# @ param: none
# @ output: Status text
# @ output-type: text/plain
status:
	@echo "ok"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), parser.ParseOptions{
		Delimiter: "@",
		Strict:    true,
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Fatal("expected validation errors for missing strict-mode annotations")
	}

	for _, key := range []string{"read-only", "destructive", "idempotent", "open-world"} {
		if !validationErrorContains(result.Errors, key) {
			t.Fatalf("expected validation error for %q, got: %v", key, result.Errors)
		}
	}
}

func TestParseMakefile_MultipleParams(t *testing.T) {
	input := `
# @ name: Greet User
# @ description: Sends a personalised greeting.
# @ risk: low
# @ param: name string | The name to include in the greeting
# @ param: count int | Number of times to repeat (default: 1)
# @ param: shout bool | Whether to use uppercase
# @ output: One greeting line per repetition
# @ output-type: text/plain
greet:
	@echo "Hello, $(NAME)!"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	r := result.Recipes[0]
	if len(r.Params) != 3 {
		t.Fatalf("expected 3 params, got %d: %v", len(r.Params), r.Params)
	}
	want := []struct {
		name  string
		ptype parser.ParamType
	}{
		{"name", parser.ParamTypeString},
		{"count", parser.ParamTypeInt},
		{"shout", parser.ParamTypeBool},
	}
	for i, w := range want {
		if r.Params[i].Name != w.name {
			t.Errorf("param[%d] Name: got %q, want %q", i, r.Params[i].Name, w.name)
		}
		if r.Params[i].Type != w.ptype {
			t.Errorf("param[%d] Type: got %q, want %q", i, r.Params[i].Type, w.ptype)
		}
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_NoParamDeclared(t *testing.T) {
	// No @param annotation at all → Params is nil → validation error.
	input := `
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	if result.Recipes[0].Params != nil {
		t.Error("expected Params to be nil when @param is absent")
	}
	if result.Valid() {
		t.Error("expected validation error for missing @param declaration, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "param") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a param-related validation error, got: %v", result.Errors)
	}
}

func TestParseMakefile_InvalidParamType(t *testing.T) {
	input := `
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: count float | A float param
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected error for invalid param type, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "float") || strings.Contains(e.Error(), "type") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a type-related error, got: %v", result.Errors)
	}
}

func TestParseMakefile_InvalidParamFormat(t *testing.T) {
	// Missing the ' | ' separator — should produce a parse error.
	input := `
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: name string The description without pipe
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected error for malformed @param value, got none")
	}
}

func TestParseMakefile_ParamNoneMixedWithNamed(t *testing.T) {
	input := `
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: none
# @ param: name string | A name
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected error for mixing @param: none with named params, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "mix") || strings.Contains(e.Error(), "none") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a mix-related error, got: %v", result.Errors)
	}
}

// ----- ParseMakefile — general tests -----

func TestParseMakefile_HelloWorld(t *testing.T) {
	input := `
# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
hello-world:
	@echo "Hello, World!"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	r := result.Recipes[0]
	if r.ID != "hello-world" {
		t.Errorf("ID: got %q, want %q", r.ID, "hello-world")
	}
	if r.Name != "Hello World" {
		t.Errorf("Name: got %q, want %q", r.Name, "Hello World")
	}
	if r.Risk != parser.RiskLow {
		t.Errorf("Risk: got %q, want %q", r.Risk, parser.RiskLow)
	}
	if r.OutputType != "text/plain" {
		t.Errorf("OutputType: got %q, want %q", r.OutputType, "text/plain")
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_MultipleRecipes(t *testing.T) {
	input := `
# @ name: Hello World
# @ description: Greets the world.
# @ risk: low
# @ param: none
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"

# @ name: Clean
# @ description: Removes build artifacts.
# @ risk: medium
# @ param: none
# @ output: confirmation
# @ output-type: text/plain
clean:
	@rm -rf ./bin
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 2 {
		t.Fatalf("expected 2 recipes, got %d", len(result.Recipes))
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_MissingMetadataFields(t *testing.T) {
	// Recipe missing output-type; all other fields including @param: none are present.
	input := `
# @ name: Hello World
# @ description: Greets the world.
# @ risk: low
# @ param: none
# @ output: greeting
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected validation errors for missing output-type, got none")
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 validation error, got %d: %v", len(result.Errors), result.Errors)
	}
}

func TestParseMakefile_AllViolationsCollected(t *testing.T) {
	// Recipe has only 'name' set — missing description, params, output, output-type,
	// and has an invalid risk. All violations must appear in result.Errors.
	input := `
# @ name: Hello World
# @ risk: extreme
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	if len(result.Errors) < 2 {
		t.Errorf("expected multiple validation errors (all violations), got %d: %v",
			len(result.Errors), result.Errors)
	}
}

func TestParseMakefile_InvalidRiskInFile(t *testing.T) {
	input := `
# @ name: Danger
# @ description: Does something risky.
# @ risk: extreme
# @ param: none
# @ output: chaos
# @ output-type: text/plain
danger:
	@rm -rf /
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected validation error for invalid risk level, got none")
	}
}

func TestParseMakefile_InvalidOutputTypeInFile(t *testing.T) {
	input := `
# @ name: Emit JSON
# @ description: Prints JSON.
# @ risk: low
# @ param: none
# @ output: JSON payload
# @ output-type: definitely not valid
emit-json:
	@echo "{}"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Fatal("expected validation error for invalid output-type, got none")
	}
	found := false
	for _, validationErr := range result.Errors {
		if strings.Contains(validationErr.Error(), "valid HTTP content type") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected output-type validation error, got %v", result.Errors)
	}
}

func TestParseMakefile_RecipeWithoutMetadata(t *testing.T) {
	input := `
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 0 {
		t.Errorf("expected 0 recipes for unannotated target, got %d", len(result.Recipes))
	}
	if !result.Valid() {
		t.Errorf("expected no errors for unannotated target, got: %v", result.Errors)
	}
}

func TestParseMakefile_AnnotationWithoutSpaceRejected(t *testing.T) {
	// Old format: no space between delimiter and key — must be rejected.
	input := `
# @name: Hello World
# @description: Greets.
# @risk: low
# @param: none
# @output: greeting
# @output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 0 {
		t.Errorf("expected 0 recipes (old format should be rejected), got %d", len(result.Recipes))
	}
}

func TestParseMakefile_VariableAssignmentsIgnored(t *testing.T) {
	input := `
SHELL=/usr/bin/env sh
VERSION=1.0.0

# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: none
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Errorf("expected 1 recipe, got %d", len(result.Recipes))
	}
}

func TestParseMakefile_PhonyIgnored(t *testing.T) {
	input := `
.PHONY: hello-world

# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: none
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Errorf("expected 1 recipe (not .PHONY), got %d", len(result.Recipes))
	}
	if result.Recipes[0].ID != "hello-world" {
		t.Errorf("expected recipe ID 'hello-world', got %q", result.Recipes[0].ID)
	}
}

func TestParseMakefile_EmptyInput(t *testing.T) {
	result, err := parser.ParseMakefile(strings.NewReader(""), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 0 {
		t.Errorf("expected 0 recipes for empty input, got %d", len(result.Recipes))
	}
	if !result.Valid() {
		t.Errorf("expected no errors for empty input, got: %v", result.Errors)
	}
}

func TestParseMakefile_CustomDelimiter(t *testing.T) {
	input := `
# mcp name: Hello World
# mcp description: Greets using a custom delimiter.
# mcp risk: low
# mcp param: none
# mcp output: greeting
# mcp output-type: text/plain
hello-world:
	@echo "Hello"
`
	opts := parser.ParseOptions{Delimiter: "mcp"}
	result, err := parser.ParseMakefile(strings.NewReader(input), opts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	if result.Recipes[0].Name != "Hello World" {
		t.Errorf("Name: got %q, want %q", result.Recipes[0].Name, "Hello World")
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_DefaultDelimiterWhenEmpty(t *testing.T) {
	// An empty Delimiter in ParseOptions should fall back to "@".
	input := validAnnotations("hello-world")
	result, err := parser.ParseMakefile(strings.NewReader(input), parser.ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Errorf("expected 1 recipe with default delimiter, got %d", len(result.Recipes))
	}
}

func TestParseMakefile_InlineAnnotation(t *testing.T) {
	// Annotations embedded after non-comment content on the same line are
	// collected because annotation detection (extractAnnotation) is pre-computed
	// before the switch statement. If the line contains a valid annotation,
	// case isAnnotation fires first regardless of any other content on the line.
	// This means `SHELL=/bin/sh  # @ name: foo` produces a name annotation.
	input := `
SHELL=/bin/sh  # @ name: Shell Setup
# @ description: Sets the shell.
# @ risk: low
# @ param: none
# @ output: none
# @ output-type: text/plain
set-shell:
	@echo "shell set"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Errorf("expected 1 recipe from inline annotation, got %d", len(result.Recipes))
	}
	if len(result.Recipes) > 0 && result.Recipes[0].Name != "Shell Setup" {
		t.Errorf("Name: got %q, want %q", result.Recipes[0].Name, "Shell Setup")
	}
}

func TestParseMakefile_InvalidKeyRejected(t *testing.T) {
	// An annotation key that fails the key regex is silently skipped.
	input := `
# @ 123invalid: Hello World
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: none
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Errorf("expected 1 recipe, got %d", len(result.Recipes))
	}
}

// ----- ParseMakefilePath / ParseMakefiles tests -----

func TestParseMakefilePath_TestdataFile(t *testing.T) {
	result, err := parser.ParseMakefilePath("../../testdata/Makefile", defaultOpts)
	if err != nil {
		t.Fatalf("failed to open testdata/Makefile: %v", err)
	}
	if len(result.Recipes) < 1 {
		t.Fatalf("expected at least 1 recipe in testdata/Makefile, got %d", len(result.Recipes))
	}
	for _, r := range result.Recipes {
		if r.SourceFile == "" {
			t.Errorf("recipe %q: SourceFile should be set", r.ID)
		}
	}
	if !result.Valid() {
		t.Errorf("testdata/Makefile has validation errors: %v", result.Errors)
	}
}

func TestParseMakefiles_MultiplePaths(t *testing.T) {
	result, err := parser.ParseMakefiles(
		[]string{"../../testdata/Makefile", "../../testdata/Makefile"},
		defaultOpts,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both files are identical so expect double the recipe count.
	if len(result.Recipes) < 2 {
		t.Errorf("expected merged recipes from 2 files, got %d", len(result.Recipes))
	}
}

func TestParseMakefiles_MissingFileReturnsError(t *testing.T) {
	_, err := parser.ParseMakefiles([]string{"/nonexistent/Makefile"}, defaultOpts)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestValidationError_Error_WithSourceFile(t *testing.T) {
	ve := parser.ValidationError{
		RecipeID:   "hello",
		SourceFile: "/some/Makefile",
		Err:        fmt.Errorf("missing name"),
	}
	got := ve.Error()
	if !strings.Contains(got, "/some/Makefile") {
		t.Errorf("Error() = %q, want it to contain the source file path", got)
	}
}

func TestParseMakefile_ParamMissingType(t *testing.T) {
	// "@param: name | description" — only one word before the pipe; should error.
	input := `
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: name | The name to greet
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected validation error for param missing type, got none")
	}
}

func TestParseMakefile_InvalidParamName(t *testing.T) {
	// Param name starts with a digit — invalid per paramNameRE.
	input := `
# @ name: Hello World
# @ description: Greets.
# @ risk: low
# @ param: 123bad string | Bad name
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if result.Valid() {
		t.Error("expected validation error for invalid param name, got none")
	}
}

func TestParseMakefile_PlainCommentInsideAnnotationBlock(t *testing.T) {
	// A non-annotation comment line inside an annotation block must not reset
	// the block — the recipe should still be parsed correctly.
	input := `
# @ name: Hello World
# This is a plain comment, not an annotation.
# @ description: Prints a greeting.
# @ risk: low
# @ param: none
# @ output: greeting
# @ output-type: text/plain
hello-world:
	@echo "Hello"
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	if result.Recipes[0].Name != "Hello World" {
		t.Errorf("Name = %q, want %q", result.Recipes[0].Name, "Hello World")
	}
	if !result.Valid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestParseMakefile_DotTargetAfterAnnotation(t *testing.T) {
	// A dot-prefixed target (e.g. .special:) immediately after a full annotation
	// block resets state without producing a recipe.
	input := `
# @ name: Special
# @ description: A dot-prefixed internal target.
# @ risk: low
# @ param: none
# @ output: nothing
# @ output-type: text/plain
.special-internal:
	@echo noop
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 0 {
		t.Errorf("expected 0 recipes for dot-prefixed target, got %d", len(result.Recipes))
	}
}

func TestParseMakefile_BlankLineInAnnotationBlock(t *testing.T) {
	// A blank line between the annotation block and the target resets state,
	// so no recipe should be produced.
	input := "# @ name: Something\n# @ description: desc.\n# @ risk: low\n# @ param: none\n# @ output: out\n# @ output-type: text/plain\n\nsomething:\n\t@echo hi\n"
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 0 {
		t.Errorf("expected 0 recipes (blank line resets annotation state), got %d", len(result.Recipes))
	}
}

func TestParseMakefilePath_WithValidationErrors(t *testing.T) {
	// Parsing a file with an invalid risk level produces a ValidationError
	// and ParseMakefilePath sets SourceFile on each error.
	dir := t.TempDir()
	path := dir + "/Makefile"
	content := "# @ name: Bad\n# @ description: Invalid risk.\n# @ risk: extreme\n# @ param: none\n# @ output: nothing\n# @ output-type: text/plain\nbad:\n\t@echo bad\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	result, err := parser.ParseMakefilePath(path, defaultOpts)
	if err != nil {
		t.Fatalf("ParseMakefilePath() error = %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for invalid risk, got none")
	}
	for _, ve := range result.Errors {
		if ve.SourceFile == "" {
			t.Errorf("SourceFile not set on error: %v", ve)
		}
	}
}

func TestParseMakefile_TargetWithPrerequisites(t *testing.T) {
	// Target lines like "build: tests format" are valid Make; the parser should
	// use the first word as the target ID.
	input := `
# @ name: Build
# @ description: Compiles everything.
# @ risk: low
# @ param: none
# @ output: binaries
# @ output-type: application/octet-stream
build: tests format
	@go build ./...
`
	result, err := parser.ParseMakefile(strings.NewReader(input), defaultOpts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(result.Recipes))
	}
	if result.Recipes[0].ID != "build" {
		t.Errorf("ID = %q, want %q", result.Recipes[0].ID, "build")
	}
}
