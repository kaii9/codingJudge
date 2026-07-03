import { check } from 'k6';
import { listProblems, getProblem } from './lib/client.js';

export const options = {
  vus: parseInt(__ENV.K6_VUS) || 1,
  duration: __ENV.K6_DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300'],
    checks: ['rate>0.99'],
  },
};

export default function () {
  const list = listProblems();
  const problems = list.json();
  if (Array.isArray(problems) && problems.length > 0) {
    const id = problems[0].id;
    getProblem(id);
  }
}
