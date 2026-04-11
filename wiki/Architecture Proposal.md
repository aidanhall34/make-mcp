# MCP Server Architecture Proposal

## Overview

The MCP server wraps parsed Makefile recipes as MCP tools and serves them to
LLM clients over stdio or streamable HTTP. It watches the configured Makefiles
for changes, reloads on save, and pushes `notifications/tools/list_changed` to
all connected clients when the tool list updates. Every LLM-facing interaction
is traced with OpenTelemetry, and operational metrics are exported via
Prometheus or OTLP depending on the `OTEL_METRICS_EXPORTER` environment
variable.

---

## Package structure

```
cmd/
  mcp-server/           # Binary entry point: parses flags/env, wires
                        # telemetry, watcher, and server, then blocks.

pkg/
  config/               # Existing: Config struct, YAML loading, Merge.
  parser/               # Existing: Makefile parsing, Recipe schema, validation.

  telemetry/            # OpenTelemetry bootstrap.
    telemetry.go        # Init tracer + meter from env vars; returns Shutdown func.
    metrics.go          # Named metric instruments shared across packages.

  watcher/              # File-change detection.
    watcher.go          # Wraps fsnotify; debounces rapid saves; emits reload events.

  runner/               # Safe Make invocation.
    runner.go           # Executes `make <target>` with env vars from Param values.
                        # Captures stdout, stderr, exit code; enforces timeout.

  server/               # MCP protocol implementation.
    server.go           # Core server: holds live tool registry, handles reload events.
    tools.go            # Converts parser.Recipe → MCP tool definitions.
    handler.go          # JSON-RPC dispatch: initialize, tools/list, tools/call.
    transport/
      stdio.go          # Reads JSON-RPC from stdin, writes to stdout.
      http.go           # POST /mcp (client→server) + GET /mcp SSE (server→client).
```

All new packages follow [AGENTS.md](https://github.com/aidanhall34/make-mcp/blob/main/AGENTS.md): packages under `./pkg`, the binary under
`./cmd`.

---

## MCP library

Use **`github.com/mark3labs/mcp-go`** for the MCP protocol layer. It provides:

- JSON-RPC message types and dispatcher
- `Server` and `Client` abstractions
- SSE transport helpers for streamable HTTP
- `Tool`, `ToolInput`, `ToolResult` types

This keeps the protocol plumbing out of the application code. `pkg/server`
wraps it and owns the business logic (registry, reload, telemetry).

---

## Transport layer

### stdio

Suitable for clients that launch the server as a subprocess (e.g. Claude
Desktop, VS Code extensions). The process reads newline-delimited JSON-RPC
from stdin and writes to stdout. Stderr is reserved for logs.

### Streamable HTTP

Suitable for network-accessible deployments (Docker, K8s). The spec uses two
endpoints on the same path:

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/mcp` | Client → server (requests, responses) |
| `GET`  | `/mcp` | Server → client (SSE stream for notifications) |

A separate listener exposes Prometheus metrics if the Prometheus exporter is
active. Its bind address is configured via
`OTEL_EXPORTER_PROMETHEUS_HOST` and `OTEL_EXPORTER_PROMETHEUS_PORT`.

Transport is selected via `--transport stdio|http|both` (or `make-mcp.yml`
`transport:` key). Default transport is `stdio`. Both can run concurrently to
support mixed client environments. When HTTP is enabled, the default listen
address is `127.0.0.1:9378`.

---

## Tool registry

`pkg/server.Registry` is the live, in-memory representation of all tools. It
is the only mutable shared state in the server. Access is protected by an
`sync.RWMutex`.

```
Registry
  ├─ tools  []server.Tool         // converted from parser.Recipe
  └─ mu     sync.RWMutex
```

On startup the registry is populated from the initial parse. On every
successful file reload it is atomically swapped. On a failed reload the old
registry is preserved and an error is logged.

### Recipe → MCP tool conversion

| Recipe field  | MCP tool field |
|---------------|----------------|
| `ID`          | `name` |
| `Name`        | `annotations.title`; also included in tool `_meta` |
| `Description` | `description` |
| `Params`      | `inputSchema.properties` (see below) |
| `Risk`        | Included in tool `_meta`; also stored as a span/metric label |
| `Output`      | Included in tool `_meta` |
| `OutputType`  | Included in tool `_meta` as an HTTP content-type hint |
| `ToolHints.ReadOnly` | `annotations.readOnlyHint` |
| `ToolHints.Destructive` | `annotations.destructiveHint` |
| `ToolHints.Idempotent` | `annotations.idempotentHint` |
| `ToolHints.OpenWorld` | `annotations.openWorldHint` |

Tool `_meta` uses flat, namespaced keys such as
`make-mcp.recipe.output_content_type` so inspectors and clients can present
recipe metadata without scraping Markdown from the tool description.

Optional boolean recipe annotations `read-only`, `destructive`, `idempotent`,
and `open-world` override make-mcp's default MCP annotation hints. When absent,
the defaults are conservative: not read-only, destructive only for high-risk
recipes, not idempotent, and open-world.

`@param: none` produces `"inputSchema": {"type": "object", "properties": {}}`.

Param types map to JSON Schema as follows:

| `@param` type | JSON Schema type |
|---------------|-----------------|
| `string`      | `"string"` |
| `int`         | `"integer"` |
| `bool`        | `"boolean"` |

---

## File watching and reload flow

`pkg/watcher` wraps `fsnotify` and debounces rapid consecutive writes (e.g.
editor save-on-format) with a 200 ms settle window.

```
fsnotify event
  └─ debounce 200 ms
       └─ emit reload.Event{Path: "..."}
            ├─ re-parse with pkg/parser
            ├─ if INVALID:
            │    log.Error("reload failed: ...")
            │    metrics: tool_reload_total.Add(1, {file: path, status: fail})
            │    keep old registry
            └─ if VALID:
                 registry.Swap(newTools)
                 metrics: tool_reload_total.Add(1, {file: path, status: success})
                 push notifications/tools/list_changed to all SSE clients
```

The watcher also watches `make-mcp.yml` itself. If the config file changes, it
re-reads the `makefiles` list, adds any new paths to fsnotify, and triggers a
full reload.

---

## Command invocation

`pkg/runner` executes `make <target>` with param values passed as Make
variables on the command line:

```
make greet NAME=Alice COUNT=3
```

Param name → Make variable name mapping: uppercase the param name
(`name` → `NAME`, `output_file` → `OUTPUT_FILE`).

The runner must invoke `make` directly via `exec.CommandContext` (or
equivalent), not through a shell such as `sh -c`. The target name and every
`KEY=value` argument are passed as separate argv entries so user input cannot
escape into shell metacharacter interpretation at the server process boundary.
This proposal only covers safe process invocation by the server; it does not
attempt to sandbox the behavior of the Makefile recipes themselves.

Before invocation, the runner validates:

- target names are sourced from parsed recipe IDs, not raw client input
- each param key maps to a declared recipe param only
- each argv element is emitted as its own argument (`make`, `<target>`,
  `KEY=value`, ...)

Command-injection regressions at the process boundary must be covered by unit
tests, including inputs containing characters such as `;`, `&&`, `|`, `$()`,
and backticks.

### Invocation lifecycle

```
tools/call request
  └─ span: "mcp.tools.call" {tool=<id>, risk=<level>}
       ├─ validate params against inputSchema
       ├─ runner.Run(target, params, timeout)
       │    ├─ exec.CommandContext with per-risk timeout
       │    ├─ capture stdout + stderr
       │    └─ check exit code
       ├─ format result as MCP ToolResult with stdout and stderr content items
       ├─ metrics: tool_invocations_total.Add(1, {tool, risk, status})
       ├─ metrics: tool_invocation_duration.Record(elapsed, {tool, risk, status})
       └─ return ToolResult or error
```

Per-risk default timeouts (configurable in `make-mcp.yml`):

| Risk   | Default timeout |
|--------|----------------|
| `low`  | 10 min |
| `medium` | 10 min |
| `high` | 10 min |

### Tool result and error contract

Successful tool calls return an MCP `ToolResult` whose content contains both
stdout and stderr. The recipe's `output-type` is treated as metadata only: it
documents the expected successful stdout format, but the server does not enforce
that the command output conforms to it at runtime. stderr is treated as plain
text.

```json
{
  "content": [
    {
      "type": "text",
      "text": "Hello, Alice!\n"
    },
    {
      "type": "text",
      "text": ""
    }
  ]
}
```

The server treats failures as MCP errors rather than successful tool results.

- Input validation failure: return an error before spawning `make`
- Timeout: kill the child process and return an error with code
  `tool_timeout`
- Non-zero exit code: return an error with code `tool_exit_nonzero`

For timeout and non-zero exit cases, attach structured error data with:

- `exit_code` when available
- `stdout`
- `stderr`
- `timed_out`

Example non-zero exit error payload:

```json
{
  "code": "tool_exit_nonzero",
  "message": "make target failed with exit code 2",
  "data": {
    "exit_code": 2,
    "stdout": "partial stdout\n",
    "stderr": "make: *** [target] Error 2\n",
    "timed_out": false
  }
}
```

Example timeout error payload:

```json
{
  "code": "tool_timeout",
  "message": "make target exceeded timeout of 10m0s",
  "data": {
    "stdout": "partial stdout\n",
    "stderr": "",
    "timed_out": true
  }
}
```

This keeps success and failure handling unambiguous for clients: successful
tool results include both output streams, while failure details are attached to
the error object.

---

## OpenTelemetry instrumentation

### Bootstrap (`pkg/telemetry`)

On startup, `telemetry.Init()` reads the standard OTEL environment variables
and returns a `Shutdown` function to be deferred in `main`:

```go
shutdown, err := telemetry.Init(ctx)
defer shutdown(ctx)
```

**Metrics exporter** is selected via `OTEL_METRICS_EXPORTER`:

| Value | Exporter |
|-------|---------|
| `prometheus` | Starts `/metrics` HTTP server bound using `OTEL_EXPORTER_PROMETHEUS_HOST` and `OTEL_EXPORTER_PROMETHEUS_PORT` |
| `otlp` | OTLP gRPC exporter; endpoint from `OTEL_EXPORTER_OTLP_ENDPOINT` |
| `none` | No metrics exported |

If unset, defaults to `prometheus`.

**Traces exporter** is selected via `OTEL_TRACES_EXPORTER`:

| Value | Exporter |
|-------|---------|
| `otlp` | OTLP gRPC exporter |
| `none` / unset | No traces exported |

### Traces

Every LLM ↔ server interaction is wrapped in a span:

| Operation | Span name | Attributes |
|-----------|-----------|-----------|
| Client connects | `mcp.initialize` | `transport` |
| Tool list requested | `mcp.tools.list` | `tool_count` |
| Tool invoked | `mcp.tools.call` | `tool.name`, `tool.risk`, `tool.id` |
| File reload triggered | `mcp.reload` | `file.path` |
| Make execution | `runner.exec` | `tool.id`, `tool.risk`, `exit_code` |

`runner.exec` is a child span of `mcp.tools.call`, giving end-to-end latency
attribution.

### Metrics

All metrics live under the `make_mcp` namespace.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `make_mcp_tools_listed_total` | Counter | — | Number of tools returned in each `tools/list` response. Tracks the size of the served tool set, not the call count. |
| `make_mcp_tools_list_requests_total` | Counter | — | Number of times a client has called `tools/list`. |
| `make_mcp_tools_list_duration_seconds` | Histogram | — | Latency of `tools/list` requests. Mirrors the `mcp.tools.list` trace span. |
| `make_mcp_tool_reload_total` | Counter | `file`, `status` | Tool list reloads from file change; `status` is `success` or `fail`. |
| `make_mcp_tool_invocations_total` | Counter | `tool`, `risk`, `status` | Total tool calls; `status` is `success` or `fail`. |
| `make_mcp_tool_invocation_duration_seconds` | Histogram | `tool`, `risk`, `status` | End-to-end latency of a tool call including Make execution; `status` is `success` or `fail`. |
| `make_mcp_connected_clients` | UpDownCounter | `transport` | Current number of connected clients per transport type. |

---

## Implemented additions

### Health and readiness endpoints (HTTP transport)

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Liveness probe — always 200 if process is running. |
| `GET /ready` | Readiness probe — 200 when at least one Makefile has been parsed successfully; 503 otherwise. |

Useful for Docker health checks and Kubernetes probes.

### `make-mcp.yml` live reload

The watcher also watches `make-mcp.yml` itself (when `--config` is supplied).
If `delimiter` changes, all Makefiles are re-parsed with the new delimiter.
If the `makefiles` list changes, new paths are added to fsnotify and dropped
paths are removed.

---

## Planned features

### Audit log

Every tool invocation is written to a structured log line (JSON) including:

- Timestamp, trace ID, span ID
- Tool ID, risk level
- Exit code, duration, stdout length

This is separate from application logs and is always emitted regardless of
`OTEL_TRACES_EXPORTER`.

### Output streaming

Rather than buffering the full Make output before responding, stream stdout
chunks back as multiple MCP `content` blocks. This improves perceived latency
for long-running commands. Requires the `streamable HTTP` transport; falls back
to buffered on stdio.

---

## Configuration additions (`make-mcp.yml`)

```yaml
# Transport: stdio | http | both
transport: stdio

# HTTP listen address
listen: "127.0.0.1:9378"

# Per-risk execution timeouts
timeouts:
  low:    10m
  medium: 10m
  high:   10m
```

Telemetry export is configured with the standard OpenTelemetry environment
variables such as `OTEL_METRICS_EXPORTER`, `OTEL_TRACES_EXPORTER`,
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_PROMETHEUS_HOST`, and
`OTEL_EXPORTER_PROMETHEUS_PORT`. The YAML file does not add make-mcp-specific
OpenTelemetry keys.

All keys above have equivalent CLI flags on `cmd/mcp-server`.

---

## Dependency additions

| Package | Purpose |
|---------|---------|
| `github.com/mark3labs/mcp-go` | MCP protocol (JSON-RPC, tool types, SSE transport) |
| `github.com/fsnotify/fsnotify` | Cross-platform file watching |
| `go.opentelemetry.io/otel` | Core OTel API |
| `go.opentelemetry.io/otel/sdk` | OTel SDK (tracer/meter providers) |
| `go.opentelemetry.io/otel/exporters/prometheus` | Prometheus metrics exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` | OTLP metrics exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | OTLP trace exporter |
