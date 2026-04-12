import { group } from 'k6';
import { check } from 'k6';

import { callTool, firstText } from './helpers.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1.0'],
  },
};

export function runStreamingCallSuite() {
  group('streamable-http / tools/call slow-stream (success)', () => {
    const result = callTool('slow-stream', {});
    check(result, {
      'result is not null': (value) => value !== null && value !== undefined,
      'result has a content array': (value) =>
        value !== null && Array.isArray(value.content) && value.content.length > 0,
      'output contains chunk 1': (value) => firstText(value !== null ? value.content : []).includes('chunk 1'),
      'output contains done': (value) => firstText(value !== null ? value.content : []).includes('done'),
      'result is not flagged as error': (value) => value !== null && value.isError !== true,
    });
  });

  group('streamable-http / tools/call slow-stream-fail (failure)', () => {
    const result = callTool('slow-stream-fail', {});
    check(result, {
      'result is not null': (value) => value !== null && value !== undefined,
      'result has a content array': (value) =>
        value !== null && Array.isArray(value.content) && value.content.length > 0,
      // Partial output produced before exit must be present.
      'output contains chunk 1': (value) =>
        value !== null && value.content.some((c) => typeof c.text === 'string' && c.text.includes('chunk 1')),
      // The runner wraps make failures as isError results (MCP spec §Tool Error Handling).
      'result is flagged as error': (value) => value !== null && value.isError === true,
    });
  });
}

export default function () {
  runStreamingCallSuite();
}
