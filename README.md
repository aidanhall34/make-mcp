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

Metadata for each recipe is provided as structured comments placed **directly
above** the recipe target line with no blank lines between the block and the
target.

The declaration must be attached to the real recipe target, not to helper lines
such as `.PHONY`. Unannotated targets are ignored, so this is an intentional
tradeoff: make-mcp only discovers recipes you explicitly annotate.

### Comment format

```
.*#<delimiter> <key>: <value>
```

| Part | Description |
|---|---|
| `.*` | Optional prefix — allows annotations on the same line as other content (inline comments). |
| `#` | The Makefile comment character. |
| `<delimiter>` | A **literal string** configured via `make-mcp.yml` or `--delimiter`. Default: `@`. |
| ` ` | **One or more spaces** must separate the delimiter from the key. |
| `<key>` | A string matching `[a-zA-Z][a-zA-Z0-9\-]*`. |
| `<value>` | The rest of the line after the first `:`. |

### Required annotation fields

| Key | Repeatable | Description |
|---|---|---|
| `name` | No | Human-friendly tool name shown to the MCP client. |
| `description` | No | What the recipe does and when to use it. |
| `risk` | No | Impact level if the recipe is run: `low`, `medium`, or `high`. |
| `param` | Yes | One entry per input, or `@param: none` if the recipe takes no inputs. At least one `@param` line is required. |
| `output` | No | What the recipe produces on success. |
| `output-type` | No | MIME type of the output, e.g. `text/plain`, `application/json`. |

Recipes that are missing any required field, have an invalid `risk` value, or
have a malformed `@param` line are reported as validation errors.

Unannotated targets are silently ignored — they are not exposed as MCP tools.

### Param value format

Inputs are declared with one `@param` line per argument. The value field has
the fixed format `<name> <type> | <description>`:

```
<name> <type> | <description>
```

| Part | Description |
|---|---|
| `<name>` | Identifier for the input; matches `[a-zA-Z][a-zA-Z0-9_-]*`. Maps to the environment variable or Make variable the recipe reads. |
| `<type>` | One of `string`, `int`, `bool`. |
| `\|` | Literal pipe character separating type from description. Must be surrounded by spaces. |
| `<description>` | Free text describing the input, including any default value or constraints. |

Use `@param: none` to declare that the recipe takes no inputs. This is required;
omitting all `@param` lines is a validation error. `@param: none` and named
`@param` lines cannot be mixed in the same recipe.

### Example — no inputs

```makefile
.PHONY: hello-world

# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
hello-world:
	@echo "Hello, World!"
```

The annotation block must be placed between `.PHONY` and the target. If it
appears before `.PHONY`, the annotation is consumed by `.PHONY` and the recipe
is not discovered as an MCP tool:

```makefile
# Incorrect — annotation is consumed by .PHONY, recipe is not discovered
# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
.PHONY: hello-world
hello-world:
	@echo "Hello, World!"
```

### Example — with inputs

Multiple `@param` lines are allowed:

```makefile
# @ name: Greet User
# @ description: Sends a personalised greeting to stdout.
# @ risk: low
# @ param: name string | The name to include in the greeting
# @ param: count int | Number of times to repeat the greeting (default: 1)
# @ output: One greeting line per repetition
# @ output-type: text/plain
greet:
	@for i in $(shell seq 1 $(COUNT)); do echo "Hello, $(NAME)!"; done
```

### Optional MCP tool hints

The MCP `annotations` object can advertise behavior hints to clients. These
values are hints for clients and inspectors; make-mcp does not use them for
access control or sandboxing.

Set `strict: true` or pass `--strict` to require all four hint annotations on
every annotated recipe.

| Key | Default | Description |
|---|---:|---|
| `read-only` | `false` | `true` means the recipe is expected not to modify files, services, external state, or other environment state. |
| `destructive` | `true` for `risk: high`; otherwise `false` | `true` means the recipe may perform destructive updates such as deletion, replacement, shutdown, or rollback. |
| `idempotent` | `false` | `true` means repeated calls with the same arguments are expected to have no additional effect. |
| `open-world` | `true` | `true` means the recipe may interact with external systems such as networks, containers, cloud APIs, package registries, or databases. |

Example:

```makefile
# @ name: Test
# @ description: Runs the unit-test suite.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results
# @ output-type: text/plain
tests:
	go test -race ./...
```

## Configuration

All configuration can come from a `make-mcp.yml` file, command-line flags, or
both. **CLI flags always take precedence over the config file.**

### make-mcp.yml

```yaml
# Annotation delimiter — literal string between # and the annotation key.
# Default: "@"
delimiter: "@"

# Ordered list of Makefile paths to parse.
makefiles:
  - ./makefile
  - ./other/Makefile

# Require optional recipe annotations such as read-only/destructive hints.
strict: false

# Transport for the MCP server.
# One of: stdio, http, both
# Default: "stdio"
transport: "stdio"

# HTTP listen address when HTTP transport is enabled.
# Default: "127.0.0.1:9378"
listen: "127.0.0.1:9378"

# Per-risk execution timeouts.
# Default: 10m for all risk levels.
timeouts:
  low: 10m
  medium: 10m
  high: 10m
```

OpenTelemetry exporter behavior is configured with the standard environment
variables such as `OTEL_METRICS_EXPORTER`, `OTEL_TRACES_EXPORTER`,
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_PROMETHEUS_HOST`, and
`OTEL_EXPORTER_PROMETHEUS_PORT`.

### CLI flags

Both `cmd/validate` and `cmd/mcp-server` accept these flags:

| Flag | Equivalent config key | Description |
|---|---|---|
| `--config <path>` | — | Path to `make-mcp.yml`. |
| `--delimiter <str>` | `delimiter` | Annotation delimiter string. |
| `--makefile <path>` | `makefiles` | Makefile to parse; repeatable for multiple files. |
| `--strict` | `strict` | Require every supported recipe annotation, including optional MCP tool hints. |

`cmd/mcp-server` additionally accepts:

| Flag | Equivalent config key | Description |
|---|---|---|
| `--transport <mode>` | `transport` | Transport to enable: `stdio`, `http`, or `both`. |
| `--listen <addr>` | `listen` | HTTP listen address. |

#### Examples

```sh
# Validate with a config file
go run ./cmd/validate --config make-mcp.yml

# Validate two Makefiles explicitly, overriding the delimiter
go run ./cmd/validate --makefile ./makefile --makefile ./other/Makefile --delimiter "mcp"

# Mix config file with a CLI override
go run ./cmd/validate --config make-mcp.yml --delimiter "##"
```

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
- [Testing](https://github.com/aidanhall34/make-mcp/wiki/Testing)
- [GitHub CI](https://github.com/aidanhall34/make-mcp/wiki/GitHub-CI)
- [Architecture](https://github.com/aidanhall34/make-mcp/wiki/Architecture)
- [Output Types](https://github.com/aidanhall34/make-mcp/wiki/Output-Types)
- [Metrics](https://github.com/aidanhall34/make-mcp/wiki/Metrics)

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
