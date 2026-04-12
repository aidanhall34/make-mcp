import { group } from 'k6';

import { checkToolListResponse, listAllTools } from './helpers.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1.0'],
  },
};

export function runToolsListSuite() {
  group('streamable-http / tools/list', () => {
    const response = listAllTools();
    checkToolListResponse(response.tools);
  });
}

export default function () {
  runToolsListSuite();
}
