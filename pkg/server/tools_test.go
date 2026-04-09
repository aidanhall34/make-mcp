package server

import (
	"encoding/json"
	"testing"

	"github.com/aidanhall34/make-mcp/pkg/parser"
)

func TestRecipeToToolReturnsStructuredMetadata(t *testing.T) {
	readOnly := true
	destructive := false
	idempotent := true
	openWorld := false

	tool, err := recipeToTool(parser.Recipe{
		ID:          "emit-json",
		Name:        "Emit JSON",
		Description: "Prints JSON to stdout.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A JSON object",
		OutputType:  "application/json",
		ToolHints: parser.ToolHints{
			ReadOnly:    &readOnly,
			Destructive: &destructive,
			Idempotent:  &idempotent,
			OpenWorld:   &openWorld,
		},
	})
	if err != nil {
		t.Fatalf("recipeToTool() error = %v", err)
	}

	if tool.Description != "Prints JSON to stdout." {
		t.Fatalf("tool description = %q, want plain recipe description", tool.Description)
	}
	if tool.Annotations.Title != "Emit JSON" {
		t.Fatalf("tool annotation title = %q, want recipe name", tool.Annotations.Title)
	}
	assertBoolPtr(t, tool.Annotations.ReadOnlyHint, true, "ReadOnlyHint")
	assertBoolPtr(t, tool.Annotations.DestructiveHint, false, "DestructiveHint")
	assertBoolPtr(t, tool.Annotations.IdempotentHint, true, "IdempotentHint")
	assertBoolPtr(t, tool.Annotations.OpenWorldHint, false, "OpenWorldHint")
	if tool.Meta == nil {
		t.Fatal("tool Meta = nil, want recipe metadata")
	}

	meta := tool.Meta.AdditionalFields
	assertMetaString(t, meta, "make-mcp.recipe.id", "emit-json")
	assertMetaString(t, meta, "make-mcp.recipe.name", "Emit JSON")
	assertMetaString(t, meta, "make-mcp.recipe.risk", "low")
	assertMetaString(t, meta, "make-mcp.recipe.output", "A JSON object")
	assertMetaString(t, meta, "make-mcp.recipe.output_content_type", "application/json")
}

func TestRecipeToToolUsesToolAnnotationDefaults(t *testing.T) {
	tool, err := recipeToTool(parser.Recipe{
		ID:          "deploy",
		Name:        "Deploy",
		Description: "Deploys the app.",
		Risk:        parser.RiskHigh,
		Params:      []parser.Param{},
		Output:      "Deploy logs",
		OutputType:  "text/plain",
	})
	if err != nil {
		t.Fatalf("recipeToTool() error = %v", err)
	}

	assertBoolPtr(t, tool.Annotations.ReadOnlyHint, false, "ReadOnlyHint")
	assertBoolPtr(t, tool.Annotations.DestructiveHint, true, "DestructiveHint")
	assertBoolPtr(t, tool.Annotations.IdempotentHint, false, "IdempotentHint")
	assertBoolPtr(t, tool.Annotations.OpenWorldHint, true, "OpenWorldHint")
}

func TestRecipeToToolMarshalsMetadataAsMetaField(t *testing.T) {
	tool, err := recipeToTool(parser.Recipe{
		ID:          "emit-json",
		Name:        "Emit JSON",
		Description: "Prints JSON to stdout.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "A JSON object",
		OutputType:  "application/json",
	})
	if err != nil {
		t.Fatalf("recipeToTool() error = %v", err)
	}

	raw, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal tool: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal tool: %v", err)
	}
	meta, ok := got["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("tool JSON _meta = %T, want object; raw=%s", got["_meta"], raw)
	}
	if meta["make-mcp.recipe.output_content_type"] != "application/json" {
		t.Fatalf("tool JSON _meta output type = %v", meta["make-mcp.recipe.output_content_type"])
	}
}

func assertMetaString(t *testing.T, meta map[string]any, key, want string) {
	t.Helper()

	if got, ok := meta[key].(string); !ok || got != want {
		t.Fatalf("meta[%q] = %v (%T), want %q", key, meta[key], meta[key], want)
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
