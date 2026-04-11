import { group } from 'k6';
import http from 'k6/http';
import { check } from 'k6';

import { BASE_URL } from './helpers.js';

const REQUEST_TIMEOUT = __ENV.MCP_HTTP_TIMEOUT || '30s';
const MCP_PROTOCOL_VERSION = '2024-11-05';
const SESSION_HEADER = 'Mcp-Session-Id';

function params(tags, extraHeaders) {
  return {
    headers: {
      'content-type': 'application/json',
      accept: 'application/json',
      ...extraHeaders,
    },
    timeout: REQUEST_TIMEOUT,
    tags,
  };
}

function jsonRPC(method, paramsObject) {
  return JSON.stringify({
    jsonrpc: '2.0',
    id: `${__VU}-${__ITER}-${method}`,
    method,
    params: paramsObject,
  });
}

function initializeSession() {
  const response = http.post(
    BASE_URL,
    jsonRPC('initialize', {
      protocolVersion: MCP_PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: { name: 'make-mcp-k6', version: '0.1.0' },
    }),
    params({ mcp_method: 'initialize' }),
  );
  check(response, { 'initialize status is 200': (r) => r.status === 200 });
  const sessionId = response.headers[SESSION_HEADER];

  http.post(
    BASE_URL,
    JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized', params: {} }),
    params({ mcp_method: 'notifications/initialized' }, { [SESSION_HEADER]: sessionId }),
  );

  return sessionId;
}

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1.0'],
  },
};

export function runErrorComplianceSuite() {
  group('streamable-http / error compliance', () => {
    group('calling a non-existent tool returns a JSON-RPC error', () => {
      const sessionId = initializeSession();
      const response = http.post(
        BASE_URL,
        jsonRPC('tools/call', { name: 'tool-that-does-not-exist', arguments: {} }),
        params({ mcp_method: 'tools/call', tool_name: 'tool-that-does-not-exist' }, {
          [SESSION_HEADER]: sessionId,
        }),
      );
      check(response, {
        'status is 200': (r) => r.status === 200,
        'response contains a JSON-RPC error': (r) => {
          const payload = r.json();
          return payload !== null && payload.error !== undefined && payload.error !== null;
        },
        // mcp-go returns INVALID_PARAMS (-32602) for unknown tools; -32601 is
        // METHOD_NOT_FOUND which is reserved for unknown JSON-RPC methods.
        'error code indicates invalid params': (r) => {
          const payload = r.json();
          return payload !== null && payload.error !== null && payload.error.code === -32602;
        },
      });
    });

    group('calling always-fail returns an isError tool result', () => {
      const sessionId = initializeSession();
      const response = http.post(
        BASE_URL,
        jsonRPC('tools/call', { name: 'always-fail', arguments: {} }),
        params({ mcp_method: 'tools/call', tool_name: 'always-fail' }, {
          [SESSION_HEADER]: sessionId,
        }),
      );
      check(response, {
        'status is 200': (r) => r.status === 200,
        // Make execution failures are returned as successful JSON-RPC responses
        // with result.isError = true (MCP spec §Tool Error Handling).
        'response is a successful JSON-RPC result': (r) => {
          const payload = r.json();
          return payload !== null && payload.result !== undefined && payload.result !== null;
        },
        'result has isError true': (r) => {
          const payload = r.json();
          return payload !== null && payload.result !== null && payload.result.isError === true;
        },
        'result has non-empty content': (r) => {
          const payload = r.json();
          if (payload === null || payload.result === null) return false;
          return Array.isArray(payload.result.content) && payload.result.content.length > 0;
        },
      });
    });
  });
}

export default function () {
  runErrorComplianceSuite();
}
