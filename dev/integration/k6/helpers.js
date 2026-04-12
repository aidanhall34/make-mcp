import http from 'k6/http';
import { check, fail } from 'k6';

export const BASE_URL = (__ENV.MCP_BASE_URL || 'http://localhost:9378/mcp').replace(/\/$/, '');
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

function decodeJSON(response, label) {
  const payload = response.json();
  if (!payload) {
    fail(`${label} returned an empty body`);
  }
  if (payload.error) {
    fail(`${label} failed with JSON-RPC error ${payload.error.code}: ${payload.error.message}`);
  }
  return payload;
}

function sessionHeaders(sessionId) {
  if (!sessionId) {
    return {};
  }
  return {
    [SESSION_HEADER]: sessionId,
  };
}

function initializeSession() {
  const response = http.post(
    BASE_URL,
    jsonRPC('initialize', {
      protocolVersion: MCP_PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: {
        name: 'make-mcp-k6',
        version: '0.1.0',
      },
    }),
    params({ mcp_method: 'initialize' }),
  );
  check(response, {
    'initialize status is 200': (res) => res.status === 200,
  });
  const payload = decodeJSON(response, 'initialize');
  const sessionId = response.headers[SESSION_HEADER];
  if (!sessionId) {
    fail(`initialize response missing ${SESSION_HEADER} header`);
  }

  const initialized = http.post(
    BASE_URL,
    JSON.stringify({
      jsonrpc: '2.0',
      method: 'notifications/initialized',
      params: {},
    }),
    params({ mcp_method: 'notifications/initialized' }, sessionHeaders(sessionId)),
  );
  check(initialized, {
    'initialized notification status is 200': (res) => res.status === 200 || res.status === 202,
  });

  return { sessionId, initializeResult: payload.result || {} };
}

export function listAllTools() {
  const { sessionId } = initializeSession();
  const response = http.post(
    BASE_URL,
    jsonRPC('tools/list', {}),
    params({ mcp_method: 'tools/list' }, sessionHeaders(sessionId)),
  );
  check(response, {
    'tools/list status is 200': (res) => res.status === 200,
  });
  const payload = decodeJSON(response, 'tools/list');
  return payload.result || {};
}

export function callTool(name, args) {
  const { sessionId } = initializeSession();
  const response = http.post(
    BASE_URL,
    jsonRPC('tools/call', { name, arguments: args }),
    params({ mcp_method: 'tools/call', tool_name: name }, sessionHeaders(sessionId)),
  );
  check(response, {
    'tools/call status is 200': (res) => res.status === 200,
  });
  const payload = decodeJSON(response, 'tools/call');
  return payload.result;
}

export function findTool(tools, name) {
  if (!Array.isArray(tools)) return undefined;
  return tools.find((tool) => tool.name === name);
}

export function firstText(content) {
  if (!Array.isArray(content) || content.length === 0) return '';
  const item = content.find((entry) => entry.text !== undefined);
  return item ? String(item.text || '') : '';
}

export function checkToolListResponse(tools) {
  check(tools, {
    'response is an array': (items) => Array.isArray(items),
    'at least one tool is registered': (items) => Array.isArray(items) && items.length > 0,
    'hello-world tool is present': (items) => findTool(items, 'hello-world') !== undefined,
  });

  const helloWorld = findTool(tools, 'hello-world');
  check(helloWorld, {
    'hello-world has a non-empty description': (tool) =>
      tool !== undefined && typeof tool.description === 'string' && tool.description.length > 0,
    'hello-world has an inputSchema': (tool) =>
      tool !== undefined && tool.inputSchema !== null && tool.inputSchema !== undefined,
  });
}

export function checkHelloWorldCall(result) {
  check(result, {
    'result is not null': (value) => value !== null && value !== undefined,
    'result has a content array': (value) =>
      value !== null && Array.isArray(value.content) && value.content.length > 0,
    'output contains greeting': (value) =>
      firstText(value !== null ? value.content : []).includes('Hello, World!'),
    'result is not flagged as error': (value) => value !== null && value.isError !== true,
  });
}
