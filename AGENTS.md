# make-mcp Agent Instructions

These instructions are stored in the repository root so coding agents can use
the same project context without relying on global machine configuration.

# Project summary

- make-mcp is a Go MCP server that exposes explicitly annotated Makefile
  recipes as MCP tools.
- Unannotated Makefile targets are ignored.
- the make targets to `test` and `build` must be run at the completion of every task. They must be invoked via the MCP make-mcp server.
- The server can run over stdio or HTTP, as configured in `./make-mcp.yml`.

# Common commands

- Use `make tests` to run the full Go unit-test suite.
- Use `make validate` to validate annotated recipes in the configured Makefile.
- Use `make build` to build project binaries into `./bin`.
- Use `make dev-up` to start the local LGTM/Grafana development backend.
- Use `make dev-down` to stop the local development backend.

# Docs conventions

- Documentation is written to the `./docs` folder in markdown format.
- Keep user-facing setup and annotation examples in `./README.md`.

# Makefile conventions

- All commands for interacting with the repository are stored in `./makefile`.
- Public makefile recipes are accessable through the `make-mcp` mcp server.\
  Always use this MCP server to interact with make.
- MCP recipe annotations must sit directly above the real recipe target.
- Do not put `.PHONY` between an MCP annotation block and the recipe target.
- `.PHONY` may be declared anywhere else in the Makefile.
- make recipes should be small and composable. Prefer chaining many small recipes rather than writing large recipes.\
  e.g.

  ```
  make build > make publish # Good, composable, can be written as make build publish or make build or make publish
  make publish # Bad, build and publish steps wrapped into one recipe.
  ```

- All variables must be wrapped in double quotes (")
- Use the `Lint` make-mcp server tool to validate changes made to the makefile upon every change.

# Coding conventions

- Code should be written in Go.
- Code should always include unit tests.
- Use the `Test` make-mcp server tool to validate changes made to code upon every change.
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

- All image tags should be set with environment variables.
- Validate dockerfile changes with the `Build Container` make-mcp server tool
