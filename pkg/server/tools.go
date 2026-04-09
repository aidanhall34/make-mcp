package server

import (
	"encoding/json"
	"fmt"

	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func buildServerTools(recipes []parser.Recipe, handler mcpserver.ToolHandlerFunc) ([]mcpserver.ServerTool, map[string]parser.Recipe, error) {
	tools := make([]mcpserver.ServerTool, 0, len(recipes))
	recipeIndex := make(map[string]parser.Recipe, len(recipes))

	for _, recipe := range recipes {
		tool, err := recipeToTool(recipe)
		if err != nil {
			return nil, nil, err
		}
		tools = append(tools, mcpserver.ServerTool{
			Tool:    tool,
			Handler: handler,
		})
		recipeIndex[recipe.ID] = recipe
	}

	return tools, recipeIndex, nil
}

func recipeToTool(recipe parser.Recipe) (mcp.Tool, error) {
	schema, err := buildInputSchema(recipe.Params)
	if err != nil {
		return mcp.Tool{}, err
	}
	tool := mcp.NewToolWithRawSchema(recipe.ID, toolDescription(recipe), schema)
	tool.Annotations = toolAnnotations(recipe)
	tool.Meta = mcp.NewMetaFromMap(toolMetadata(recipe))
	return tool, nil
}

func buildInputSchema(params []parser.Param) (json.RawMessage, error) {
	properties := map[string]map[string]any{}
	for _, param := range params {
		properties[param.Name] = map[string]any{
			"type":        schemaType(param.Type),
			"description": param.Description,
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal tool input schema: %w", err)
	}
	return raw, nil
}

func toolDescription(recipe parser.Recipe) string {
	return recipe.Description
}

func toolAnnotations(recipe parser.Recipe) mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		Title:           recipe.Name,
		ReadOnlyHint:    boolHint(recipe.ToolHints.ReadOnly, false),
		DestructiveHint: boolHint(recipe.ToolHints.Destructive, recipe.Risk == parser.RiskHigh),
		IdempotentHint:  boolHint(recipe.ToolHints.Idempotent, false),
		OpenWorldHint:   boolHint(recipe.ToolHints.OpenWorld, true),
	}
}

func boolHint(value *bool, defaultValue bool) *bool {
	if value != nil {
		return value
	}
	return mcp.ToBoolPtr(defaultValue)
}

func toolMetadata(recipe parser.Recipe) map[string]any {
	meta := map[string]any{
		"make-mcp.recipe.id":                  recipe.ID,
		"make-mcp.recipe.name":                recipe.Name,
		"make-mcp.recipe.risk":                string(recipe.Risk),
		"make-mcp.recipe.output":              recipe.Output,
		"make-mcp.recipe.output_content_type": recipe.OutputType,
	}
	if recipe.SourceFile != "" {
		meta["make-mcp.recipe.source_file"] = recipe.SourceFile
	}
	return meta
}

func schemaType(paramType parser.ParamType) string {
	switch paramType {
	case parser.ParamTypeInt:
		return "integer"
	case parser.ParamTypeBool:
		return "boolean"
	default:
		return "string"
	}
}
