# Telemetry Reference

> This file is auto-generated. Run `make gen-metrics-doc` to regenerate.

The MCP server exposes Prometheus metrics when `OTEL_METRICS_EXPORTER=prometheus`
(the default). The scrape endpoint is `http://127.0.0.1:9090/metrics` by default;
override with `OTEL_EXPORTER_PROMETHEUS_HOST` and `OTEL_EXPORTER_PROMETHEUS_PORT`.

---

## Counters

### `make_mcp_tools_listed_total`

| Property | Value |
|---|---|
| Description | — |
| Unit | — |
| Labels | `destructive`, `idempotent`, `open_world`, `read_only`, `risk` |

### `make_mcp_tools_list_requests_total`

| Property | Value |
|---|---|
| Description | — |
| Unit | — |
| Labels | `status` |

### `make_mcp_tool_reload_total`

| Property | Value |
|---|---|
| Description | — |
| Unit | — |
| Labels | `file`, `status` |

### `make_mcp_tool_invocations_total`

| Property | Value |
|---|---|
| Description | — |
| Unit | — |
| Labels | `destructive`, `idempotent`, `open_world`, `read_only`, `risk`, `status`, `streaming`, `tool` |

### `make_mcp_auth_attempts_total`

| Property | Value |
|---|---|
| Description | Total number of OAuth token validation attempts. |
| Unit | — |
| Labels | `status` |

### `make_mcp_resources_listed_total`

| Property | Value |
|---|---|
| Description | Total number of resources returned across all resources/list requests. |
| Unit | — |
| Labels | `status` |

### `make_mcp_resources_read_total`

| Property | Value |
|---|---|
| Description | Total number of resources/read requests. |
| Unit | — |
| Labels | `status` |

### `make_mcp_files_watched_total`

| Property | Value |
|---|---|
| Description | Total number of files ever registered as MCP resources (including removed). |
| Unit | — |
| Labels | _(none)_ |

### `make_mcp_resource_subscriptions_total`

| Property | Value |
|---|---|
| Description | Total number of resource subscription events (subscribe/unsubscribe). |
| Unit | — |
| Labels | `action` |

## Histograms

### `make_mcp_tools_list_request_latency_seconds`

| Property | Value |
|---|---|
| Description | Server-side latency of tools/list requests. |
| Unit | s |
| Labels | `status` |

### `make_mcp_tools_list_request_bytes_in`

| Property | Value |
|---|---|
| Description | Bytes received in tools/list requests (cursor size). |
| Unit | By |
| Labels | `status` |

### `make_mcp_tools_list_request_bytes_out`

| Property | Value |
|---|---|
| Description | Bytes sent in tools/list responses (JSON-encoded tool definitions). |
| Unit | By |
| Labels | `status` |

### `make_mcp_tool_invocation_duration_seconds`

| Property | Value |
|---|---|
| Description | — |
| Unit | — |
| Labels | `destructive`, `idempotent`, `open_world`, `read_only`, `risk`, `status`, `streaming`, `tool` |

### `make_mcp_tool_invocation_bytes_in`

| Property | Value |
|---|---|
| Description | Bytes received in tool invocation requests (JSON-encoded arguments). |
| Unit | By |
| Labels | `destructive`, `idempotent`, `open_world`, `read_only`, `risk`, `status`, `streaming`, `tool` |

### `make_mcp_tool_invocation_bytes_out`

| Property | Value |
|---|---|
| Description | Bytes sent in tool invocation responses (stdout + stderr). |
| Unit | By |
| Labels | `destructive`, `idempotent`, `open_world`, `read_only`, `risk`, `status`, `streaming`, `tool` |

### `make_mcp_auth_attempt_duration_seconds`

| Property | Value |
|---|---|
| Description | Latency of OAuth token validation attempts. |
| Unit | s |
| Labels | `status` |

### `make_mcp_resources_list_request_duration_seconds`

| Property | Value |
|---|---|
| Description | Server-side latency of resources/list requests. |
| Unit | s |
| Labels | `status` |

### `make_mcp_resources_list_file_count`

| Property | Value |
|---|---|
| Description | Number of resources returned per resources/list request. |
| Unit | — |
| Labels | `status` |

### `make_mcp_resources_read_duration_seconds`

| Property | Value |
|---|---|
| Description | Server-side latency of resources/read requests. |
| Unit | s |
| Labels | `status` |

### `make_mcp_resources_read_file_count`

| Property | Value |
|---|---|
| Description | Number of file contents returned per resources/read response. |
| Unit | — |
| Labels | `status` |

### `make_mcp_resources_read_bytes_in`

| Property | Value |
|---|---|
| Description | Bytes received in resources/read requests (URI length). |
| Unit | By |
| Labels | `status` |

### `make_mcp_resources_read_bytes_out`

| Property | Value |
|---|---|
| Description | Bytes sent in resources/read responses (file content size). |
| Unit | By |
| Labels | `status` |

### `make_mcp_resource_notification_duration_seconds`

| Property | Value |
|---|---|
| Description | Latency of sending resource update notifications to subscribed clients. |
| Unit | s |
| Labels | `status` |

## Gauges

### `make_mcp_connected_clients`

| Property | Value |
|---|---|
| Description | — |
| Unit | — |
| Labels | `transport` |

### `make_mcp_tools_registered`

| Property | Value |
|---|---|
| Description | Current number of registered MCP tools. |
| Unit | — |
| Labels | `destructive`, `idempotent`, `open_world`, `read_only`, `risk` |

### `make_mcp_files_watched`

| Property | Value |
|---|---|
| Description | Current number of files registered as MCP resources. |
| Unit | — |
| Labels | _(none)_ |

### `make_mcp_resource_subscriptions_active`

| Property | Value |
|---|---|
| Description | Current number of active resource subscriptions. |
| Unit | — |
| Labels | _(none)_ |

## Dashboards

A dashboard for working with this repo can be found at `./dev/dashboards/development-dashboard.json`.\
![make-mcp key metrics dashboard panels](./images/make-mcp-development-panels.png)\
It contains example panels for key metrics to monitor when running make-mcp.
