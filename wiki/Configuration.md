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

# Allowed browser origin for CORS on the HTTP /mcp endpoint.
# When empty or "*" any origin is permitted — suitable for local development
# and the MCP Inspector. In production, set this to the exact URL of your
# web client (e.g. "https://claude.ai") so the browser blocks cross-origin
# requests from all other origins. Has no effect on the stdio transport or
# on non-browser clients (curl, CLI tools, server-to-server calls).
# Default: "" (wildcard — any origin allowed)
cors_origin: ""

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

# TLS certificate paths and HTTP security settings for the HTTP server.
# When cert and key are set, the server listens on HTTPS instead of HTTP.
# tls.cert and tls.key are required when oauth.enabled is true.
# ca is optional: provide it when the JWKS endpoint (oauth.jwks_uri) uses a
# self-signed or private CA certificate that the system pool won't trust.
# cors_origin locks browser cross-origin access; empty = any origin allowed.
# tls:
#   cert:        ./dev/certs/server.crt   # PEM-encoded server certificate
#   key:         ./dev/certs/server.key   # PEM-encoded server private key
#   ca:          ./dev/certs/ca.crt       # optional — CA cert for JWKS trust
#   cors_origin: "https://claude.ai"      # optional — restrict browser origins

# OAuth 2.1 Bearer token authentication (HTTP transport only).
# tls.cert and tls.key are mandatory when oauth.enabled is true.
# oauth:
#   enabled: false
#   issuer:   "https://auth.example.com/realms/make-mcp"
#   audience: "make-mcp"
#   jwks_uri: "https://auth.example.com/realms/make-mcp/protocol/openid-connect/certs"
#   groups:
#     - name: "/admins"
#       resources:
#         - "config"
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
