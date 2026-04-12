# Testing

This document describes the full test strategy for make-mcp, covering unit
tests, benchmarks, coverage enforcement, telemetry, and integration tests.

---

## Unit tests and benchmarks

All Go packages under `./cmd` and `./pkg` have unit tests and benchmarks
co-located with the source.  Every test file follows the naming convention
`<package>_test.go` (external `_test` package) or `<package>_internal_test.go`
(same package, for white-box tests).

Run the full suite:

```sh
make tests
# equivalent: go test -race -cover -coverprofile=coverage.out ./...
```

Benchmarks are included in `_bench_test.go` files and follow the same package
convention.  They can be run selectively:

```sh
go test -bench=. -run='^$' ./pkg/parser/
```

---

## Coverage enforcement

Every package contains a `testmain_test.go` file that calls
`testtel.RunMain(m, threshold)` (or `covercheck.Report` for `internal/testtel`
itself, to avoid a circular dependency).  This enforces a per-package coverage
threshold and emits per-file JSON coverage to stderr on every run:

```json
{"file":"github.com/aidanhall34/make-mcp/pkg/runner/runner.go","coverage":1,"pass":true}
{"coverage":1,"threshold":0.95,"pass":true}
```

Thresholds are set per-package based on genuinely achievable coverage.
Packages with untestable dead code (e.g., OTLP paths that require a live
endpoint, or `cmd/*` `main()` wiring) carry lower thresholds.  Thresholds are
documented in the corresponding `testmain_test.go`.

---

## Test telemetry (OpenTelemetry)

The `internal/testtel` package wraps each test run in an OpenTelemetry trace
span and records metrics.  It is a drop-in replacement for `covercheck.Report`
inside `TestMain`:

```go
func TestMain(m *testing.M) {
    os.Exit(testtel.RunMain(m, 0.95))
}
```

Individual tests call `testtel.Start` to create a root span and propagate
context into the code under test:

```go
func TestFoo(t *testing.T) {
    ctx := testtel.Start(t) // root span; ctx carries trace context
    result, err := mycode.DoSomething(ctx, ...)
}
```

### Trace and metric instruments

| Instrument | Type | Description |
|---|---|---|
| `make_mcp_test_duration_seconds` | Histogram | Per-test duration, labelled by `test.name` and `test.status` |

### Span links to CI executor

When the `TRACEPARENT` environment variable (W3C trace context format) is set,
each root test span gets a `trace.Link` back to the executor's span.  This
allows CI/CD pipelines to correlate individual test traces with the pipeline
run that triggered them.

### Enabling telemetry during tests

Set `OTEL_EXPORTER_OTLP_ENDPOINT` before running tests.  When unset, all
telemetry is a no-op so ordinary `go test` runs are unaffected.

```sh
# With the dev LGTM stack running (make dev-up):
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 make tests
```

Inside the Docker build, the endpoint is injected at build time via
`--build-arg OTEL_EXPORTER_OTLP_ENDPOINT=...` (see `build-container` recipe).

---

## Integration tests

Integration tests verify the MCP protocol interface (streamable HTTP) from the
perspective of an external MCP client.  They are **not** load tests —
correctness and protocol compliance only.

### Tools

- **k6** — load testing tool used here for functional protocol testing
- **xk6-mcp** ([grafana/xk6-mcp](https://github.com/grafana/xk6-mcp)) — k6
  extension that provides a native MCP client for test scripts

### What is tested

| Scenario | Transport | Checks |
|---|---|---|
| `tools/list` | Streamable HTTP | Response is an array; `hello-world` tool present; tool has description and `inputSchema` |
| `tools/call hello-world` | Streamable HTTP | Content array non-empty; first item is `text` type; text includes `"Hello, World!"`; `isError` is false |

### Running integration tests

Three recipes are available depending on whether you want telemetry data from
the run:

| Recipe | LGTM stack | OTel output |
|---|---|---|
| `make integration` | not used | none |
| `make integration-lgtm` | started and stopped by the recipe | yes |
| `make integration-otel` | must already exist outside the recipe | yes, left running after |
| `make integration-act` | ensured to be up if absent, never stopped by the recipe | yes, left running after |

```sh
# Plain functional run — no telemetry, no dev stack required.
make integration

# Start LGTM, run with OTel output, stop LGTM on exit.
make integration-lgtm

# Host or external OTLP collector already available; run with OTel output and
# leave that collector alone.
make dev-up          # if not already up
make integration-otel

# Ensure the host LGTM stack is up for act-style runs, but never stop it.
make integration-act
```

All three recipes:

1. Build the server container (`make build`) — unit tests run inside the Docker
   build and must pass.
2. Build the custom `k6+xk6-mcp` image (`make integration-build-k6`).
3. Create `./dev/tmp/` for runtime artefacts.
4. Start the MCP server container with HTTP transport and wait for `/ready` to
   return 200.
5. Run the k6 test suite via `docker compose run`.
6. Tear down the integration containers and remove `./dev/tmp/` on exit
   (always, on success or failure).

`integration-lgtm` additionally stops the full local dev stack in the EXIT trap.
`integration-otel` and `integration-act` leave the existing collector running.

Exit code mirrors k6's exit code: 0 = all checks passed, non-zero = failure.

### OpenTelemetry metrics from k6

`make integration-lgtm`, `make integration-otel`, and `make integration-act`
automatically set the k6 OTel env vars and route output to the configured OTLP
collector. By default that collector is the host LGTM stack on port 4317. The
host gateway is added to the integration containers via
`dev/docker-compose.integration.telemetry-host.yml` so they can reach
`host.docker.internal:4317` when needed.

### File layout

```
dev/
  docker-compose.lgtm.yml          LGTM-only development stack
  docker-compose.grafana-mcp.yml   Optional Grafana MCP sidecar for local development
  docker-compose.integration.server.yml
  docker-compose.integration.runner.yml
  docker-compose.integration.telemetry-host.yml
  integration/
    make-mcp.yml                   Server config (HTTP transport, test fixture)
    k6/
      Dockerfile                   Builds k6 + xk6-mcp from source
      integration.js               Runs the full local integration suite
      tools-list.js                Focused tools/list integration case
      tools-call-hello-world.js    Focused tools/call integration case
  tmp/                             Runtime artifacts (created + deleted per run)
    summary.json                   k6 JSON summary output
testdata/
  Makefile                         Annotated Makefile served by the integration server
```

### CI integration

The `ci.yml` GitHub Actions workflow runs the focused k6 scripts as a parallel
matrix after unit tests complete. Under `act`, those jobs use
`make integration-act` so telemetry is forwarded to the host LGTM collector
without stopping it and without touching the optional Grafana MCP sidecar. On
GitHub-hosted runners, the same jobs use the standard
`OTEL_EXPORTER_OTLP_ENDPOINT` secret when it is configured; otherwise telemetry
is disabled by setting the exporters to `none`.

---

## Byte metrics

Production metrics track bytes transferred across all HTTP interfaces:

| Metric | Unit | Labels |
|---|---|---|
| `make_mcp_tools_list_request_bytes_in` | bytes | `status` |
| `make_mcp_tools_list_request_bytes_out` | bytes | `status` |
| `make_mcp_tool_invocation_bytes_in` | bytes | `tool`, `status` |
| `make_mcp_tool_invocation_bytes_out` | bytes | `tool`, `status` |

`bytes_in` for `tools/list` is the cursor length (pagination); `bytes_out` is
the JSON size of the returned tool definitions.  For invocations, `bytes_in` is
the JSON-encoded argument map size; `bytes_out` is `len(stdout) + len(stderr)`.

---

## Future work — load testing

> **Note:** Load testing is explicitly out of scope for the current integration
> test suite.  A separate load testing phase (with and without attached AI
> models) will be designed later using k6 scenarios and the same xk6-mcp
> extension.  The integration test infrastructure (k6 image, compose stack,
> OTel export) is intentionally built to be reused for load testing with
> minimal changes.
