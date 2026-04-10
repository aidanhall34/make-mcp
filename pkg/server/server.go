package server

import (
	"context"
	"encoding/json"
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
	// listStarts tracks the wall-clock start time for each in-flight tools/list
	// request, keyed by the opaque request ID supplied by the mcp-go hooks.
	var listStarts sync.Map

	hooks := &mcpserver.Hooks{}
	hooks.AddBeforeListTools(func(_ context.Context, id any, _ *mcp.ListToolsRequest) {
		listStarts.Store(id, time.Now())
	})
	hooks.AddAfterListTools(func(ctx context.Context, id any, req *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		if v, ok := listStarts.LoadAndDelete(id); ok {
			telemetry.RecordToolsListRequest(ctx, telemetry.StatusSuccess, time.Since(v.(time.Time)))
		}
		if result == nil {
			return
		}
		bytesIn, bytesOut := measureListBytes(req.Params.Cursor, result.Tools)
		telemetry.RecordToolsListBytes(ctx, telemetry.StatusSuccess, bytesIn, bytesOut)
	})

	s := &ToolServer{
		cfg:      cfg,
		mcp:      mcpserver.NewMCPServer("make-mcp", "dev", mcpserver.WithToolCapabilities(true), mcpserver.WithHooks(hooks)),
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

	args := request.GetArguments()
	bytesIn := int64(0)
	if encoded, err := json.Marshal(args); err == nil {
		bytesIn = int64(len(encoded))
	}

	start := time.Now()
	result, err := runner.Run(callCtx, runner.Request{
		Recipe:  recipe,
		Args:    args,
		Timeout: s.timeoutFor(recipe.Risk),
	})
	elapsed := time.Since(start)
	status := telemetry.StatusSuccess
	bytesOut := int64(0)
	if err != nil {
		status = telemetry.StatusFailure
		if runnerErr, ok := err.(*runner.Error); ok {
			bytesOut = int64(len(runnerErr.Stdout) + len(runnerErr.Stderr))
		}
	} else {
		bytesOut = int64(len(result.Stdout) + len(result.Stderr))
	}
	s.recordInvocationMetrics(callCtx, recipe, status, elapsed, bytesIn, bytesOut)
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

func (s *ToolServer) recordInvocationMetrics(ctx context.Context, recipe parser.Recipe, status string, elapsed time.Duration, bytesIn, bytesOut int64) {
	telemetry.RecordToolInvocation(ctx, recipe, status, elapsed)
	telemetry.RecordToolInvocationBytes(ctx, recipe, status, bytesIn, bytesOut)
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

// measureListBytes returns bytes_in (cursor length) and bytes_out (JSON-encoded
// size of the tools slice) for a tools/list exchange.
func measureListBytes(cursor mcp.Cursor, tools []mcp.Tool) (bytesIn, bytesOut int64) {
	bytesIn = int64(len(cursor))
	if encoded, err := json.Marshal(tools); err == nil {
		bytesOut = int64(len(encoded))
	}
	return
}
