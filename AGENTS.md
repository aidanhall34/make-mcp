# make-mcp Agent Instructions

These instructions are stored in the repository root so coding agents can use
the same project context without relying on global machine configuration.

# CRITICAL: always use the MCP server — never Bash for make

**Never run `make`, `go run ./cmd/validate`, or any makefile target via the Bash tool.**
Use the `make-mcp` MCP server for every makefile interaction. The server is
always connected and all annotated targets are available as MCP tools.

| Action | MCP tool to call |
|---|---|
| After ALL changes | `lint` |
| After any code change | `tests` |
| Full pre-merge check | `lint` `tests` |
| Build binaries | `build-mcp-server`, `build-validator` |
| Build container | `build-container` |
| Start dev stack | `dev-up` |
| Stop dev stack | `dev-down` |

# Project summary

- make-mcp is a Go MCP server that exposes explicitly annotated Makefile
  recipes as MCP tools.
- Unannotated Makefile targets are ignored.
- The server can run over stdio or HTTP, as configured in `./make-mcp.yml`.

# Common commands (MCP tools only)

- `unit-tests` — run the full Go unit-test suite.
- `validate` — validate annotated recipes in the configured Makefile.
- `build` — build all project binaries and containers.
- `dev-up` — start the local LGTM/Grafana development backend.
- `dev-down` — stop the local development backend.

# Docs conventions

- Documentation is written to the `./wiki` folder in GitHub Wiki format.
- Keep user-facing setup and annotation examples in `./README.md`.

# Makefile conventions

- All commands for interacting with the repository are stored in `./makefile`.
- MCP recipe annotations must sit directly above the real recipe target.
- Do not put `.PHONY` between an MCP annotation block and the recipe target. `.PHONY` may be declared anywhere else in the Makefile.
- make recipes should be small and composable. Prefer chaining many small recipes rather than writing large recipes.\
  e.g.

  ```
  make build > make publish # Good, composable, can be written as make build publish or make build or make publish
  make publish # Bad, build and publish steps wrapped into one recipe.
  ```

- All variables must be wrapped in double quotes (")

# Coding conventions

- Code should be written in Go.
- Code should always include unit tests.
- Code should be written in TDD style.
- Preserve the stdlib-only parser unless a dependency is clearly justified.
- Every package has a `testmain_test.go` with a `TestMain` that calls
  `covercheck.Report(m, threshold)` from `internal/covercheck`. This enforces
  a per-package statement-coverage threshold and emits JSON per-file coverage
  to stderr on every test run, regardless of where tests are invoked.
- Coverage thresholds are set per-package based on what is actually achievable
  (accounting for dead error paths such as `main()`, `json.Marshal` failures,
  and OTel instrument-creation errors). Thresholds for packages with untestable
  dead code (e.g. `cmd/*`, `pkg/telemetry`) are lower than 0.95.
- When adding a new package, add a `testmain_test.go` with an appropriate
  threshold (start at `0.95` and adjust down only if genuinely unreachable code
  prevents it).

# Go conventions

- Commands are written to the `./cmd` directory.
- Packages are written to the `./pkg` directory.
- Use `gofmt` on changed Go files.
- Go tests contain both `tests` and `benchmarks`

# Schema conventions

- Schemas are defined as JSON schema.

# Output type conventions

- `@ output-type` must be a valid HTTP content type, for example `text/plain`
  or `application/json; charset=utf-8`.
- Treat `@ output-type` as a type hint/metadata field.
- Do not validate command stdout/stderr against `@ output-type` at runtime.

# Telemetry conventions

- Use standard OpenTelemetry environment variables with the `OTEL_` prefix.
- Local LGTM OTLP gRPC is exposed at `http://localhost:4317` when the dev stack
  is running.
- The repo-local MCP client configuration is `./.mcp.json`.

# Docker conventions

- All image tags must be set with environment variables.
