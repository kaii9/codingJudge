import { check } from 'k6';
import { Trend } from 'k6/metrics';
import { createSubmission, pollUntilTerminal, findSumProblem } from './lib/client.js';
import { byLanguage } from './lib/programs.js';

const judgeTerminalDuration = new Trend('judge_terminal_duration');

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.K6_RATE) || 5,
      timeUnit: '1s',
      duration: __ENV.K6_DURATION || '30s',
      preAllocatedVUs: parseInt(__ENV.K6_VUS) || 10,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500'],
    logical_failures: ['rate<0.01'],
  },
};

export default function () {
  const problem = findSumProblem();
  if (!problem) return;

  const lang = Math.random() < 0.5 ? 'go' : 'python';
  const code = byLanguage(lang);

  const sub = createSubmission(problem.id, lang, code);
  if (!sub || !sub.id) return;

  const result = pollUntilTerminal(sub.id);
  if (result) {
    judgeTerminalDuration.add(result.elapsed);
  }
}
