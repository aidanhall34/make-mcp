import { group } from 'k6';

import { callTool, checkHelloWorldCall } from './helpers.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1.0'],
  },
};

export function runHelloWorldCallSuite() {
  group('streamable-http / tools/call hello-world', () => {
    const result = callTool('hello-world', {});
    checkHelloWorldCall(result);
  });
}

export default function () {
  runHelloWorldCallSuite();
}
