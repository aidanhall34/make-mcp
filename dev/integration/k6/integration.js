/**
 * make-mcp MCP protocol integration tests
 *
 * Tests the streamable HTTP transport of the make-mcp server from the
 * perspective of an MCP client. This is a
 * functional correctness suite — not a load test.  All checks must pass
 * for the run to succeed (exit 0).
 *
 * Configuration (environment variables):
 *   MCP_BASE_URL  Full MCP endpoint URL (default: http://localhost:9378/mcp)
 *
 * OpenTelemetry output and propagation:
 *   K6_OUT=experimental-opentelemetry           enable OTel metrics output
 *   K6_OTEL_GRPC_EXPORTER_ENDPOINT=host:4317   gRPC collector endpoint
 *   K6_OTEL_GRPC_EXPORTER_INSECURE=true        disable TLS for local collectors
 *   K6_OTEL_SERVICE_NAME=make-mcp-integration  override the reported service name
 *
 * The helper layer uses k6 HTTP tracing instrumentation so requests to the MCP
 * server carry W3C trace context headers automatically.
 *
 * Usage:
 *   # Docker (via make integration)
 *   make integration
 *
 *   # Local with OTel metrics (dev stack must be running)
 *   K6_OUT=experimental-opentelemetry \
 *     K6_OTEL_GRPC_EXPORTER_ENDPOINT=localhost:4317 \
 *     K6_OTEL_GRPC_EXPORTER_INSECURE=true \
 *     MCP_BASE_URL=http://localhost:9378/mcp \
 *     k6 run dev/integration/k6/integration.js
 */

import { runHelloWorldCallSuite } from './tools-call-hello-world.js';
import { options as toolsListOptions, runToolsListSuite } from './tools-list.js';
import { runErrorComplianceSuite } from './tools-call-error.js';
import { runStreamingCallSuite } from './tools-call-streaming.js';

export const options = toolsListOptions;

export default function () {
  runToolsListSuite();
  runHelloWorldCallSuite();
  runErrorComplianceSuite();
  runStreamingCallSuite();
}
