import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

export const BASE_URL = __ENV.BASE_URL || 'http://api:8080';
export const JUDGE_TIMEOUT = parseInt(__ENV.CJ_JUDGE_TIMEOUT_SECONDS) || 30;

export const logicalFailure = new Rate('logical_failures');
// 自定义指标：区分 HTTP 请求量与业务提交量。
export const submissionsCreated = new Counter('submissions_created');
export const submissionsAccepted = new Counter('submissions_accepted');

let authenticated = false;
const runID = (__ENV.CJ_RUN_ID || Date.now().toString(36))
  .replace(/[^A-Za-z0-9_-]/g, '')
  .slice(0, 16);

export function ensureAuthenticated() {
  if (authenticated) return true;

  const username = `k6_${runID}_${__VU}`.slice(0, 32);
  const password = 'correct-password';
  const payload = JSON.stringify({ username, password });
  const params = { headers: { 'Content-Type': 'application/json' } };
  const register = http.post(`${BASE_URL}/auth/register`, payload, params);
  if (register.status !== 201) {
    const login = http.post(`${BASE_URL}/auth/login`, payload, params);
    if (login.status !== 200) {
      check(login, { 'load-test authentication succeeds': () => false });
      logicalFailure.add(1);
      return false;
    }
  }
  authenticated = true;
  return true;
}

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
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': `k6-${runID}-${__VU}-${__ITER}-${Date.now()}`,
    },
  });
  check(res, { 'create submission 202': (r) => r.status === 202 });
  if (res.status !== 202) {
    logicalFailure.add(1);
    return null;
  }
  logicalFailure.add(0);
  submissionsCreated.add(1);
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
    if (['accepted', 'wrong_answer', 'compile_error', 'runtime_error', 'time_limit_exceeded', 'internal_error'].includes(status)) {
      if (status === 'accepted') {
        logicalFailure.add(0);
        submissionsAccepted.add(1);
      } else {
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

// findSumProblem 在题目列表中查找 id 为 "sum" 的 A+B 题目。
// 找不到时记录 logical failure 并返回 null——不回退到其他题目。
export function findSumProblem() {
  const res = listProblems();
  try {
    const problems = res.json();
    if (Array.isArray(problems)) {
      const sum = problems.find((p) => p.id === 'sum');
      if (sum) return sum;
    }
  } catch (_) { /* fall through */ }
  logicalFailure.add(1);
  return null;
}
