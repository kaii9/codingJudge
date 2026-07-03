import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

export const BASE_URL = __ENV.BASE_URL || 'http://api:8080';
export const JUDGE_TIMEOUT = parseInt(__ENV.JUDGE_TIMEOUT_SECONDS) || 30;

export const logicalFailure = new Rate('logical_failures');

export function listProblems() {
  const res = http.get(`${BASE_URL}/problems`);
  check(res, { 'list problems 200': (r) => r.status === 200 });
  return res;
}

export function getProblem(id) {
  const res = http.get(`${BASE_URL}/problems/${id}`);
  check(res, { 'get problem 200': (r) => r.status === 200 });
  return res;
}

export function createSubmission(problemId, language, code) {
  const payload = JSON.stringify({ problemId, language, code });
  const res = http.post(`${BASE_URL}/submissions`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });
  check(res, { 'create submission 202': (r) => r.status === 202 });
  if (res.status !== 202) {
    logicalFailure.add(1);
    return null;
  }
  try {
    return res.json();
  } catch (_) {
    logicalFailure.add(1);
    return null;
  }
}

export function pollUntilTerminal(submissionId) {
  const start = Date.now();
  while (true) {
    const res = http.get(`${BASE_URL}/submissions/${submissionId}`);
    if (res.status !== 200) {
      logicalFailure.add(1);
      return null;
    }
    const sub = res.json();
    const status = sub.status;
    if (['accepted', 'wrong_answer', 'runtime_error', 'time_limit_exceeded', 'internal_error'].includes(status)) {
      if (status !== 'accepted') {
        logicalFailure.add(1);
      }
      return { sub, elapsed: Date.now() - start };
    }
    if (Date.now() - start > JUDGE_TIMEOUT * 1000) {
      logicalFailure.add(1);
      return null;
    }
    sleep(0.1);
  }
}

// findSumProblem 在题目列表中查找 id 为 "sum" 的 A+B 题目，
// 确保 k6 提交的 A+B 代码不会被发送到不兼容的题目。
export function findSumProblem() {
  const res = listProblems();
  try {
    const problems = res.json();
    if (Array.isArray(problems)) {
      const sum = problems.find((p) => p.id === 'sum');
      if (sum) return sum;
      return problems[0];
    }
  } catch (_) { /* fall through */ }
  return null;
}
