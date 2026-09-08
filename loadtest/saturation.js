import { createSubmission, ensureAuthenticated } from './lib/client.js';

const pythonEcho = `
print(input())
# k6-saturation-test
`.trim();

export const options = {
  noCookiesReset: true,
  scenarios: {
    saturation_batch: {
      executor: 'shared-iterations',
      vus: parseInt(__ENV.CJ_VUS) || 20,
      iterations: parseInt(__ENV.CJ_ITERATIONS) || 60,
      maxDuration: __ENV.CJ_MAX_DURATION || '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    logical_failures: ['rate<0.01'],
  },
};

export default function () {
  if (!ensureAuthenticated()) return;
  createSubmission('echo', 'python', pythonEcho);
}
