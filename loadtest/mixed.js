import { createSubmission, pollUntilTerminal, listProblems, getProblem } from './lib/client.js';
import { byLanguage } from './lib/programs.js';

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
  const list = listProblems();
  const problems = list.json();
  if (!Array.isArray(problems) || problems.length === 0) return;

  // Deterministic 80% reads, 20% submissions.
  if (Math.random() < 0.8) {
    getProblem(problems[0].id);
    return;
  }

  const lang = Math.random() < 0.5 ? 'go' : 'python';
  const code = byLanguage(lang);
  const sub = createSubmission(problems[0].id, lang, code);
  if (sub && sub.id) {
    pollUntilTerminal(sub.id);
  }
}
