# make-mcp

An MCP (Model Context Protocol) server that exposes Makefile recipes as tools.
Teach your large language model to use your repo through Makefile annotations.

Teach your LLM how to understand common processes defined in makefiles, and provide a gateway for contained code execution.

![Claude acting on a natural language prompt to lint and build the repo](./wiki/images/claude_prompt.png)

---

**Connect to `make-mcp` over streamable HTTP or stdio.** Drop the connection config into your MCP client and you're ready to go:

![Claudes MCP configuration](./wiki/images/mcp_config.png)

---

**Adding more tools is as simple as annotating more recipes.** Large language models can write and share recipes with any MCP-compatible client. The server pushes an updated tool list to the client whenever a Makefile changes:

![Claudes list of tools](./wiki/images/claude_tools.png)

`make-mcp` listens for requests over stdio or streamable HTTP\
and serves provides context and execution capabilities for recipes defined in one or more Makefiles.

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

See the [Installation](https://github.com/aidanhall34/make-mcp/wiki/Installation) page for instructions to install from GitHub
Releases, Docker, `go install`, or build from source.

## Documentation

For more detailed information, see the wiki:

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
