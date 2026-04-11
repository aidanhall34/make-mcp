package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	"github.com/aidanhall34/make-mcp/pkg/runner"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

// listEntry holds per-request state threaded between BeforeListTools and
// AfterListTools hooks.
type listEntry struct {
	span  trace.Span
	start time.Time
}

// New constructs a new ToolServer from parsed recipes.
func New(cfg config.Config, recipes []parser.Recipe) (*ToolServer, error) {
	// Create s first so that hook closures can reference s.registry.
	s := &ToolServer{
		cfg:      cfg,
		registry: &Registry{},
	}

	// listStarts tracks in-flight tools/list requests, keyed by the opaque
	// request ID supplied by the mcp-go hooks.
	var listStarts sync.Map

	hooks := &mcpserver.Hooks{}
	hooks.AddBeforeListTools(func(ctx context.Context, id any, req *mcp.ListToolsRequest) {
		spanAttrs := []trace.SpanStartOption{}
		if req != nil && req.Params.Cursor != "" {
			spanAttrs = append(spanAttrs, trace.WithAttributes(
				attribute.String("tools.cursor", string(req.Params.Cursor)),
			))
		}
		_, span := telemetry.Tracer().Start(ctx, "mcp.tools.list", spanAttrs...)
		listStarts.Store(id, listEntry{span: span, start: time.Now()})
		slog.InfoContext(ctx, "tools/list request",
			"cursor", func() string {
				if req != nil {
					return string(req.Params.Cursor)
				}
				return ""
			}(),
		)
	})
	hooks.AddAfterListTools(func(ctx context.Context, id any, req *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		if v, ok := listStarts.LoadAndDelete(id); ok {
			entry := v.(listEntry)
			toolCount := 0
			var toolNames []string
			if result != nil {
				toolCount = len(result.Tools)
				for _, t := range result.Tools {
					toolNames = append(toolNames, t.Name)
				}
			}
			entry.span.SetAttributes(attribute.Int("tools.count", toolCount))
			entry.span.End()
			telemetry.RecordToolsListRequest(ctx, telemetry.StatusSuccess, time.Since(entry.start))
			telemetry.RecordToolsListed(ctx, s.registry.getRecipesByNames(toolNames))
			slog.InfoContext(ctx, "tools/list complete", "tool_count", toolCount)
		}
		if result == nil {
			return
		}
		bytesIn, bytesOut := measureListBytes(req.Params.Cursor, result.Tools)
		telemetry.RecordToolsListBytes(ctx, telemetry.StatusSuccess, bytesIn, bytesOut)
	})

	s.mcp = mcpserver.NewMCPServer("make-mcp", "dev", mcpserver.WithToolCapabilities(true), mcpserver.WithHooks(hooks))

	tools, recipeIndex, err := buildServerTools(recipes, s.handleToolCall)
	if err != nil {
		return nil, err
	}
	s.registry.swap(tools, recipeIndex)
	s.mcp.SetTools(tools...)
	telemetry.RecordToolsRegistered(context.Background(), recipes)
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
	telemetry.RecordToolsRegistered(ctx, recipes)
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
			attribute.Bool("tool.read_only", resolveHint(recipe.ToolHints.ReadOnly, false)),
			attribute.Bool("tool.destructive", resolveHint(recipe.ToolHints.Destructive, recipe.Risk == parser.RiskHigh)),
			attribute.Bool("tool.idempotent", resolveHint(recipe.ToolHints.Idempotent, false)),
			attribute.Bool("tool.open_world", resolveHint(recipe.ToolHints.OpenWorld, true)),
		),
	)
	defer span.End()

	args := request.GetArguments()
	bytesIn := int64(0)
	if encoded, err := json.Marshal(args); err == nil {
		bytesIn = int64(len(encoded))
	}

	// Add arg count to the span; individual values are added in debug mode only.
	span.SetAttributes(attribute.Int("tool.arg_count", len(args)))
	if s.cfg.Debug {
		for k, v := range args {
			span.SetAttributes(attribute.String("tool.arg."+k, fmt.Sprintf("%v", v)))
		}
	}

	if s.cfg.Debug {
		slog.DebugContext(callCtx, "tool call",
			"tool.id", recipe.ID,
			"tool.name", recipe.Name,
			"tool.risk", string(recipe.Risk),
			"arg_count", len(args),
		)
		span.AddEvent("tool.invocation", trace.WithAttributes(
			attribute.String("tool.id", recipe.ID),
			attribute.String("tool.name", recipe.Name),
			attribute.Int("tool.arg_count", len(args)),
		))
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
			eventAttrs := []attribute.KeyValue{
				attribute.String("make.error.code", runnerErr.Code),
				attribute.Bool("make.timed_out", runnerErr.TimedOut),
			}
			if runnerErr.ExitCode != nil {
				eventAttrs = append(eventAttrs, attribute.Int("make.exit_code", *runnerErr.ExitCode))
			}
			span.AddEvent("make.execution.failed", trace.WithAttributes(eventAttrs...))
			span.SetStatus(codes.Error, runnerErr.Message)
			slog.WarnContext(callCtx, "tool call failed",
				"tool.id", recipe.ID,
				"error_code", runnerErr.Code,
				"timed_out", runnerErr.TimedOut,
				"elapsed_ms", elapsed.Milliseconds(),
			)
			if s.cfg.Debug {
				slog.DebugContext(callCtx, "tool failure output",
					"tool.id", recipe.ID,
					"stdout", runnerErr.Stdout,
					"stderr", runnerErr.Stderr,
				)
			}
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					mcp.NewTextContent(runnerErr.Stdout),
					mcp.NewTextContent(runnerErr.Stderr),
				},
			}, nil
		}
		span.SetStatus(codes.Error, err.Error())
		slog.ErrorContext(callCtx, "tool call error", "tool.id", recipe.ID, "error", err.Error())
		return nil, err
	}

	if s.cfg.Debug {
		slog.DebugContext(callCtx, "tool call complete",
			"tool.id", recipe.ID,
			"elapsed_ms", elapsed.Milliseconds(),
			"stdout", result.Stdout,
			"stderr", result.Stderr,
		)
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

// resolveHint returns the value of a *bool tool hint, using defaultValue when
// the hint is nil (not explicitly annotated on the recipe).
func resolveHint(hint *bool, defaultValue bool) bool {
	if hint != nil {
		return *hint
	}
	return defaultValue
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

func (r *Registry) getRecipesByNames(names []string) []parser.Recipe {
	r.mu.RLock()
	defer r.mu.RUnlock()
	recipes := make([]parser.Recipe, 0, len(names))
	for _, name := range names {
		if recipe, ok := r.recipes[name]; ok {
			recipes = append(recipes, recipe)
		}
	}
	return recipes
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
