# make-mcp

An MCP (Model Context Protocol) server that exposes Makefile recipes as tools.

## Project goal

`make-mcp` starts as a Docker container or background binary. It listens for MCP
requests over stdio or streamable HTTP and serves information about the recipes
defined in one or more Makefiles.

Each annotated recipe is exposed as a distinct MCP tool. The server:

- Parses Makefiles on startup and whenever a file changes.
- Notifies connected clients when the tool list changes (MCP `tools/list_changed` notification).
- Returns full recipe metadata to the client before any tool is invoked.
- Invokes recipes as `make <target> KEY=value ...` using Make variables, not shell-expanded command strings.

## Annotation schema

See [[Schema]] for the full annotation reference, including field definitions, param format, output types, and optional MCP tool hints.

## Configuration

See [[Configuration]] for `make-mcp.yml` options, CLI flags, and OpenTelemetry environment variables.

## Execution and error semantics

When a tool is invoked, the server runs `make <target> KEY=value ...`, where
each declared input becomes an uppercased Make variable. Inputs are passed as
discrete process arguments, not interpolated into a shell command string.

On success, the MCP tool result contains both stdout and stderr. On failure,
the server returns an MCP error instead of a successful tool result.

- Validation errors fail before `make` is started.
- Timeouts fail with a structured timeout error.
- Non-zero exit codes fail with a structured error that includes the exit code,
  stdout, and stderr.

## Installation

See [[Installation]] for instructions to install from GitHub
Releases, Docker, `go install`, or build from source.

## Documentation

For more detailed information, see:

- [[Installation]]
- [[Schema]]
- [[Configuration]]
- [[Testing]]
- [[GitHub CI]]
- [[Architecture]]
- [[Metrics]]

## Development

```sh
# Install git hooks and Node.js dev dependencies
make setup

# Run all tests
make tests

# Build both binaries into ./bin/
make build

# Validate the project Makefile
make validate

# Lint everything
make lint
```

Tests are written before implementation (TDD). Run `make tests` after every
change to a Go source file.
