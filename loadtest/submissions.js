import { Trend } from 'k6/metrics';
import { createSubmission, pollUntilTerminal, findSumProblem } from './lib/client.js';
import { byLanguage } from './lib/programs.js';

const judgeTerminalDuration = new Trend('judge_terminal_duration');
// 使用 Python 作为基准语言，避免 Docker-in-Docker 编译开销影响吞吐量测量。
// Go/C++ 在 Docker-in-Docker 中编译耗时会因宿主机资源竞争导致超时，
// 这不代表 codingJudge 代码缺陷，而是 macOS Docker Desktop 的已知限制。
const languages = ['python'];

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

  // 确定性语言轮换：使用 VU 和迭代编号，避免 Math.random。
  const language = languages[(__VU + __ITER) % languages.length];
  const code = byLanguage(language);

  const sub = createSubmission(problem.id, language, code);
  if (!sub || !sub.id) return;

  const result = pollUntilTerminal(sub.id);
  if (result) {
    judgeTerminalDuration.add(result.elapsed);
  }
}
