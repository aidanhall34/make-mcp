package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/aidanhall34/make-mcp/pkg/runner"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const toolListChangedMethod = "notifications/tools/list_changed"

// Registry is the live in-memory tool registry.
type Registry struct {
	mu      sync.RWMutex
	tools   []mcpserver.ServerTool
	recipes map[string]parser.Recipe
}

// ToolServer wraps the underlying MCP server with make-specific behavior.
type ToolServer struct {
	cfg      config.Config
	mcp      *mcpserver.MCPServer
	registry *Registry
}

// New constructs a new ToolServer from parsed recipes.
func New(cfg config.Config, recipes []parser.Recipe) (*ToolServer, error) {
	s := &ToolServer{
		cfg:      cfg,
		mcp:      mcpserver.NewMCPServer("make-mcp", "dev", mcpserver.WithToolCapabilities(true)),
		registry: &Registry{},
	}

	tools, recipeIndex, err := buildServerTools(recipes, s.handleToolCall)
	if err != nil {
		return nil, err
	}
	s.registry.swap(tools, recipeIndex)
	s.mcp.SetTools(tools...)
	return s, nil
}

// MCP exposes the underlying library server for transport integration.
func (s *ToolServer) MCP() *mcpserver.MCPServer {
	return s.mcp
}

// Ready reports whether at least one tool is registered.
func (s *ToolServer) Ready() bool {
	s.registry.mu.RLock()
	defer s.registry.mu.RUnlock()
	return len(s.registry.tools) > 0
}

// Reload replaces the registered tools and notifies connected clients.
func (s *ToolServer) Reload(ctx context.Context, recipes []parser.Recipe, changedFile string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "mcp.reload",
		trace.WithAttributes(attribute.String("file.path", changedFile)),
	)
	defer span.End()

	tools, recipeIndex, err := buildServerTools(recipes, s.handleToolCall)
	if err != nil {
		telemetry.RecordToolReload(ctx, changedFile, telemetry.StatusFailure)
		return err
	}

	s.registry.swap(tools, recipeIndex)
	s.mcp.SetTools(tools...)
	s.mcp.SendNotificationToAllClients(toolListChangedMethod, map[string]any{})
	telemetry.RecordToolReload(ctx, changedFile, telemetry.StatusSuccess)
	return nil
}

func (s *ToolServer) handleToolCall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	recipe, ok := s.registry.getRecipe(request.Params.Name)
	if !ok {
		return nil, jsonRPCError(-32601, fmt.Sprintf("tool %q not found", request.Params.Name), nil)
	}

	callCtx, span := telemetry.Tracer().Start(ctx, "mcp.tools.call",
		trace.WithAttributes(
			attribute.String("tool.name", recipe.Name),
			attribute.String("tool.id", recipe.ID),
			attribute.String("tool.risk", string(recipe.Risk)),
		),
	)
	defer span.End()

	start := time.Now()
	result, err := runner.Run(callCtx, runner.Request{
		Recipe:  recipe,
		Args:    request.GetArguments(),
		Timeout: s.timeoutFor(recipe.Risk),
	})
	status := telemetry.StatusSuccess
	if err != nil {
		status = telemetry.StatusFailure
	}
	s.recordInvocationMetrics(callCtx, recipe, status, time.Since(start))
	if err != nil {
		if runnerErr, ok := err.(*runner.Error); ok {
			return nil, runnerErrorToJSONRPC(runnerErr)
		}
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(result.Stdout),
			mcp.NewTextContent(result.Stderr),
		},
	}, nil
}

func (s *ToolServer) recordInvocationMetrics(ctx context.Context, recipe parser.Recipe, status string, elapsed time.Duration) {
	telemetry.RecordToolInvocation(ctx, recipe, status, elapsed)
}

func (s *ToolServer) timeoutFor(risk parser.RiskLevel) time.Duration {
	switch risk {
	case parser.RiskHigh:
		return s.cfg.Timeouts.High
	case parser.RiskMedium:
		return s.cfg.Timeouts.Medium
	default:
		return s.cfg.Timeouts.Low
	}
}

func (r *Registry) swap(tools []mcpserver.ServerTool, recipes map[string]parser.Recipe) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = tools
	r.recipes = recipes
}

func (r *Registry) getRecipe(name string) (parser.Recipe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	recipe, ok := r.recipes[name]
	return recipe, ok
}
