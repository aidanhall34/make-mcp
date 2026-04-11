# Architecture

## Overview

make-mcp is an MCP server that wraps annotated Makefile recipes as MCP tools and
serves them to LLM clients over stdio or streamable HTTP. It watches configured
Makefiles for changes, reloads on save, and pushes `notifications/tools/list_changed`
to all connected clients when the tool list updates. Every LLM-facing interaction
is traced with OpenTelemetry, and operational metrics are exported via Prometheus
or OTLP depending on the `OTEL_METRICS_EXPORTER` environment variable.

---

## Package structure

```
cmd/
  mcp-server/           # Binary entry point: parses flags/env, wires
                        # telemetry, watcher, and server, then blocks.
  validate/             # CLI tool for validating annotated recipes without
                        # starting the server.

pkg/
  config/               # Config struct, YAML loading, Merge.
  parser/               # Makefile parsing, Recipe schema, validation.

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
      http.go           # Streamable HTTP MCP at /mcp, health endpoints.

internal/
  covercheck/           # Per-package test coverage enforcement.
  testtel/              # In-process OTel provider for use in tests.
```

All packages follow the project conventions in [AGENTS.md](https://github.com/aidanhall34/make-mcp/blob/main/AGENTS.md):
application packages under `./pkg`, binaries under `./cmd`, and test-only
helpers under `./internal`.

---

## MCP library

make-mcp uses **`github.com/mark3labs/mcp-go`** for the MCP protocol layer. It
provides:

- JSON-RPC message types and dispatcher
- `Server` and `Client` abstractions
- SSE transport helpers for streamable HTTP
- `Tool`, `ToolInput`, `ToolResult` types

`pkg/server` wraps this library and owns the application logic: registry
management, live reload, and telemetry.

---

## Transport layer

### stdio

Suitable for clients that launch the server as a subprocess (e.g. Claude
Desktop, VS Code extensions). The process reads newline-delimited JSON-RPC
from stdin and writes to stdout. Stderr is reserved for logs.

### Streamable HTTP

Suitable for network-accessible deployments (Docker, Kubernetes). The MCP
endpoint follows the streamable HTTP spec:

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/mcp` | Client → server (requests, responses) |
| `GET`  | `/mcp` | Server → client (SSE stream for notifications) |

The HTTP transport also exposes health and readiness endpoints on the same
listener:

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Liveness probe — always `200` if the process is running. |
| `GET /ready` | Readiness probe — `200` once at least one Makefile has been parsed successfully; `503` otherwise. |

These are useful for Docker health checks and Kubernetes probes.

A separate listener exposes Prometheus metrics when the Prometheus exporter is
active. Its bind address is configured via `OTEL_EXPORTER_PROMETHEUS_HOST` and
`OTEL_EXPORTER_PROMETHEUS_PORT` (defaults: `127.0.0.1:9090`).

Transport is selected via `--transport stdio|http|both` (or the `transport:` key
in `make-mcp.yml`). The default is `stdio`. Both transports can run concurrently.
When HTTP is enabled, the default listen address is `127.0.0.1:9378`.

---

## Tool registry

`pkg/server.Registry` is the live, in-memory representation of all tools. It is
the only mutable shared state in the server. Access is protected by a
`sync.RWMutex`.

```
Registry
  ├─ tools    []server.Tool         // converted from parser.Recipe
  ├─ recipes  map[string]parser.Recipe
  └─ mu       sync.RWMutex
```

On startup the registry is populated from the initial parse. On every successful
file reload it is atomically swapped. On a failed reload the old registry is
preserved and an error is logged.

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

## File watching and reload

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

The watcher also monitors `make-mcp.yml` itself (when `--config` is supplied).
If `delimiter` changes, all Makefiles are re-parsed with the new delimiter. If
the `makefiles` list changes, new paths are added to fsnotify and dropped paths
are removed.

---

## Command invocation

`pkg/runner` executes `make <target>` with param values passed as Make variables
on the command line:

```
make greet NAME=Alice COUNT=3
```

Param name → Make variable name mapping: uppercase the param name
(`name` → `NAME`, `output_file` → `OUTPUT_FILE`).

The runner invokes `make` directly via `exec.CommandContext`, not through a
shell such as `sh -c`. The target name and every `KEY=value` argument are passed
as separate argv entries so user input cannot escape into shell metacharacter
interpretation at the server process boundary. This only covers safe process
invocation by the server; it does not attempt to sandbox the behavior of the
Makefile recipes themselves.

Before invocation, the runner validates:

- Target names are sourced from parsed recipe IDs, not raw client input.
- Each param key maps to a declared recipe param only.
- Each argv element is emitted as its own argument (`make`, `<target>`, `KEY=value`, ...).

Command-injection regressions at the process boundary are covered by unit tests,
including inputs containing `;`, `&&`, `|`, `$()`, and backticks.

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

| Risk     | Default timeout |
|----------|----------------|
| `low`    | 10 min |
| `medium` | 10 min |
| `high`   | 10 min |

### Tool result and error contract

Successful tool calls return an MCP `ToolResult` whose content contains both
stdout and stderr. The recipe's `output-type` is treated as metadata only: it
documents the expected successful stdout format, but the server does not enforce
that command output conforms to it at runtime.

```json
{
  "content": [
    {"type": "text", "text": "Hello, Alice!\n"},
    {"type": "text", "text": ""}
  ]
}
```

Failures are returned as MCP errors rather than successful tool results:

- **Input validation failure** — error returned before spawning `make`.
- **Timeout** — child process is killed; error with code `tool_timeout`.
- **Non-zero exit code** — error with code `tool_exit_nonzero`.

For timeout and non-zero exit cases, structured error data is attached:

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

---

## OpenTelemetry instrumentation

### Bootstrap (`pkg/telemetry`)

`telemetry.Init()` reads the standard OTEL environment variables and returns a
`Shutdown` function to be deferred in `main`:

```go
shutdown, err := telemetry.Init(ctx)
defer shutdown(ctx)
```

**Metrics exporter** is selected via `OTEL_METRICS_EXPORTER`:

| Value | Exporter |
|-------|---------|
| `prometheus` (default) | Starts a `/metrics` HTTP server at `OTEL_EXPORTER_PROMETHEUS_HOST:OTEL_EXPORTER_PROMETHEUS_PORT` (default `127.0.0.1:9090`) |
| `otlp` | OTLP gRPC exporter; endpoint from `OTEL_EXPORTER_OTLP_ENDPOINT` |
| `none` | No metrics exported |

**Traces exporter** is selected via `OTEL_TRACES_EXPORTER`:

| Value | Exporter |
|-------|---------|
| `otlp` | OTLP gRPC exporter |
| `none` / unset | No traces exported |

### Traces

| Operation | Span name | Attributes |
|-----------|-----------|-----------|
| Client connects | `mcp.initialize` | `transport` |
| Tool list requested | `mcp.tools.list` | `tool_count` |
| Tool invoked | `mcp.tools.call` | `tool.name`, `tool.risk`, `tool.id` |
| File reload triggered | `mcp.reload` | `file.path` |
| Make execution | `runner.exec` | `tool.id`, `tool.risk`, `exit_code` |

`runner.exec` is a child span of `mcp.tools.call`, providing end-to-end latency
attribution.

### Metrics

All metrics live under the `make_mcp` namespace. See the [Metrics](Metrics) wiki page for the full metric reference.
