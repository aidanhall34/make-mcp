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

See the [Schema](https://github.com/aidanhall34/make-mcp/wiki/Schema) page in the wiki for the full annotation reference, including field definitions, param format, output types, and optional MCP tool hints.

## Configuration

See the [Configuration](https://github.com/aidanhall34/make-mcp/wiki/Configuration) page in the wiki for `make-mcp.yml` options, CLI flags, and OpenTelemetry environment variables.

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

See the [Installation](https://github.com/aidanhall34/make-mcp/wiki/Installation) page in the [GitHub Wiki](https://github.com/aidanhall34/make-mcp/wiki/Installation) for instructions to install from GitHub
Releases, Docker, `go install`, or build from source.

## Documentation

For more detailed information, see the [GitHub Wiki](https://github.com/aidanhall34/make-mcp/wiki):

- [Installation](https://github.com/aidanhall34/make-mcp/wiki/Installation)
- [Schema](https://github.com/aidanhall34/make-mcp/wiki/Schema)
- [Configuration](https://github.com/aidanhall34/make-mcp/wiki/Configuration)
- [Testing](https://github.com/aidanhall34/make-mcp/wiki/Testing)
- [GitHub CI](https://github.com/aidanhall34/make-mcp/wiki/GitHub-CI)
- [Architecture](https://github.com/aidanhall34/make-mcp/wiki/Architecture)
- [Telemetry](https://github.com/aidanhall34/make-mcp/wiki/Telemetry)

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
