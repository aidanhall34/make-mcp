# make-mcp Agent Instructions

These instructions are stored in the repository root so coding agents can use
the same project context without relying on global machine configuration.

# Project summary

- make-mcp is a Go MCP server that exposes explicitly annotated Makefile
  recipes as MCP tools.
- Unannotated Makefile targets are ignored.
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
- MCP recipe annotations must sit directly above the real recipe target.
- Do not put `.PHONY` between an MCP annotation block and the recipe target.
- `.PHONY` may be declared anywhere else in the Makefile.

# Coding conventions

- Code should be written in Go.
- Code should always include unit tests.
- Tests must be invoked after changes with `make tests`.
- Code should be written in TDD style.
- Preserve the stdlib-only parser unless a dependency is clearly justified.

# Go conventions

- Commands are written to the `./cmd` directory.
- Packages are written to the `./pkg` directory.
- Use `gofmt` on changed Go files.

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

# Docker compose conventions

- All image tags should be set with environment variables.
