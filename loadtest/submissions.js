import { Trend } from 'k6/metrics';
import { createSubmission, pollUntilTerminal, findSumProblem } from './lib/client.js';
import { byLanguage } from './lib/programs.js';

const judgeTerminalDuration = new Trend('judge_terminal_duration');
// 基准测试使用 Python，规避 macOS Docker Desktop 的 Docker-in-Docker 编译资源竞争。
// Go/C++ 编译在 Linux 原生 Docker 环境下正常运行，但在 macOS Docker-in-Docker
// 中多 Worker 并发编译时编译耗时可能超过判题租约。这是已知环境限制。
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

  // 确定性语言轮换。
  const language = languages[(__VU + __ITER) % languages.length];
  const code = byLanguage(language);

  const sub = createSubmission(problem.id, language, code);
  if (!sub || !sub.id) return;

  const result = pollUntilTerminal(sub.id);
  if (result) {
    judgeTerminalDuration.add(result.elapsed);
  }
}
