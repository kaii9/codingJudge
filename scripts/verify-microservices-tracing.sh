#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd "$(dirname "$0")/.." && pwd)"
api="${CJ_MICRO_API_URL:-http://127.0.0.1:28080}"
jaeger="${CJ_TRACING_URL:-http://127.0.0.1:26686}"
cookie="$(mktemp /tmp/gojudge-trace-cookie.XXXXXX)"
trace_file="$(mktemp /tmp/gojudge-trace-data.XXXXXX)"
trap 'rm -f "$cookie" "$trace_file"' EXIT
request=(curl -fsS --connect-timeout 3 --max-time 10)
user="trace_$(date +%s)_$$"
trace_id="$(openssl rand -hex 16)"
"${request[@]}" -c "$cookie" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg username "$user" '{username:$username,password:"trace-demo-local-password"}')" "$api/auth/register" >/dev/null
response="$("${request[@]}" -b "$cookie" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $user" -H "traceparent: 00-$trace_id-1111111111111111-01" \
  -d '{"problemId":"sum","language":"python","code":"a,b=map(int,input().split());print(a+b)"}' "$api/submissions")"
id="$(jq -er '.id' <<<"$response")"
status=queued
for _ in $(seq 1 120); do
  response="$("${request[@]}" -b "$cookie" "$api/submissions/$id")"
  status="$(jq -er '.status' <<<"$response")"
  case "$status" in
    accepted) break ;;
    wrong_answer|compile_error|runtime_error|time_limit_exceeded|internal_error) printf 'trace submission failed: %s\n' "$status" >&2; exit 1 ;;
  esac
  sleep 1
done
test "$status" = accepted
cd "$task_root"
for _ in $(seq 1 60); do
  if "${request[@]}" "$jaeger/api/v3/traces/$trace_id?rawTraces=true" -o "$trace_file"; then
    if report="$(go run ./scripts/trace-report -trace-id "$trace_id" < "$trace_file" 2>&1)"; then
      printf 'accepted submission=%s; collector verified trace=%s\n%s\nUI: %s/trace/%s\n' "$id" "$trace_id" "$report" "$jaeger" "$trace_id"
      exit 0
    fi
  fi
  sleep 1
done
printf 'collector trace incomplete after bounded polling: trace=%s reason=%s\n' "$trace_id" "${report:-collector unavailable}" >&2
exit 1
