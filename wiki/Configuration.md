# Configuration

All configuration can come from a `make-mcp.yml` file, command-line flags, or
both. **CLI flags always take precedence over the config file.**

## make-mcp.yml

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

# Destination for JSON logs.
# One of: stderr, or a file path.
# Default: "stderr"
log_path: "stderr"

# Enable debug-level logging.
# When true, tool call arguments, stdout, and stderr are included in logs.
# Default: false
debug: false
```

OpenTelemetry exporter behavior is configured with the standard environment
variables such as `OTEL_METRICS_EXPORTER`, `OTEL_TRACES_EXPORTER`,
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_PROMETHEUS_HOST`, and
`OTEL_EXPORTER_PROMETHEUS_PORT`.

## CLI flags

Both `cmd/validate` and `cmd/mcp-server` accept these flags:

| Flag | Equivalent config key | Description |
|---|---|---|
| `--config <path>` | — | Path to `make-mcp.yml`. |
| `--delimiter <str>` | `delimiter` | Annotation delimiter string. |
| `--makefile <path>` | `makefiles` | Makefile to parse; repeatable for multiple files. |
| `--strict` | `strict` | Require every supported recipe annotation, including optional MCP tool hints. |
| `--log-path <path>` | `log_path` | Destination for JSON logs (e.g. `stderr`, or a file path). |
| `--debug` | `debug` | Enable debug logging (includes tool args, stdout, and stderr). |

`cmd/mcp-server` additionally accepts:

| Flag | Equivalent config key | Description |
|---|---|---|
| `--transport <mode>` | `transport` | Transport to enable: `stdio`, `http`, or `both`. |
| `--listen <addr>` | `listen` | HTTP listen address. |

### Examples

```sh
# Validate with a config file
go run ./cmd/validate --config make-mcp.yml

# Validate two Makefiles explicitly, overriding the delimiter
go run ./cmd/validate --makefile ./makefile --makefile ./other/Makefile --delimiter "mcp"

# Mix config file with a CLI override
go run ./cmd/validate --config make-mcp.yml --delimiter "##"
```
