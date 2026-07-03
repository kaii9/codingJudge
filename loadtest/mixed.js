import { createSubmission, pollUntilTerminal, findSumProblem, getProblem } from './lib/client.js';
import { byLanguage } from './lib/programs.js';

const languages = ['go', 'cpp', 'python'];

export const options = {
  vus: parseInt(__ENV.K6_VUS) || 20,
  duration: __ENV.K6_DURATION || '2m',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
    logical_failures: ['rate<0.01'],
  },
};

export default function () {
  // Deterministic 80% reads, 20% submissions.
  if (Math.random() < 0.8) {
    const problem = findSumProblem();
    if (problem) {
      getProblem(problem.id);
    }
    return;
  }

  const problem = findSumProblem();
  if (!problem) return;

  // 确定性语言轮换。
  const language = languages[(__VU + __ITER) % languages.length];
  const code = byLanguage(language);
  const sub = createSubmission(problem.id, language, code);
  if (sub && sub.id) {
    pollUntilTerminal(sub.id);
  }
}
