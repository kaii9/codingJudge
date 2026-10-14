#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd "$(dirname "$0")/.." && pwd)"
compose=(docker compose -f "$task_root/deploy/compose/microservices.yml")
if [ "${CJ_MICRO_TRACING:-0}" = 1 ]; then
  compose+=(-f "$task_root/deploy/compose/microservices-tracing.yml")
fi
api="${CJ_MICRO_API_URL:-http://127.0.0.1:28080}"
cookie="$(mktemp /tmp/gojudge-micro-fault.XXXXXX)"
user="fault_$(date +%s)_$$"
cleanup() {
  rm -f "$cookie"
  # Restore the normal grace period after the forced-drain case.
  "${compose[@]}" up -d --no-deps executor executor-b >/dev/null 2>&1 || true
}
trap cleanup EXIT
curl -fsS -c "$cookie" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg username "$user" '{username:$username,password:"micro-fault-test-password"}')" "$api/auth/register" >/dev/null
code='import time
a,b=map(int,input().split())
time.sleep(0.9)
print(a+b)'
verify_recovery() {
  local mode="$1" response id active status attempts trace_id
  trace_id="$(openssl rand -hex 16)"
  "${compose[@]}" stop executor-b >/dev/null
  if [ "$mode" = forced-drain ]; then
    CJ_MICRO_EXECUTOR_SHUTDOWN_GRACE=100ms "${compose[@]}" up -d --no-deps --force-recreate --wait executor >/dev/null
  else
    "${compose[@]}" up -d --no-deps --wait executor >/dev/null
  fi
  response="$(curl -fsS -b "$cookie" -H 'Content-Type: application/json' -H "Idempotency-Key: $user-$mode" \
    -H "traceparent: 00-$trace_id-1111111111111111-01" \
    -d "$(jq -nc --arg code "$code" '{problemId:"sum",language:"python",code:$code}')" "$api/submissions")"
  id="$(jq -r '.id' <<<"$response")"
  [[ "$id" =~ ^sub-[a-f0-9-]+$ ]]
  active=""
  for _ in $(seq 1 40); do
    active="$("${compose[@]}" exec -T executor docker ps -q --filter label=gojudge.executor=gojudge-microservices-executor-a)"
    if [ -n "$active" ]; then break; fi
    sleep 0.1
  done
  test -n "$active"
  if [ "$mode" = forced-drain ]; then
    # SIGTERM reaches the real process; grace expiry cancels its in-flight RPC.
    "${compose[@]}" stop --timeout 15 executor >/dev/null
  else
    "${compose[@]}" kill -s KILL executor >/dev/null
  fi
  "${compose[@]}" start executor-b >/dev/null
  status=queued
  for _ in $(seq 1 120); do
    response="$(curl -fsS -b "$cookie" "$api/submissions/$id")"
    status="$(jq -r '.status' <<<"$response")"
    case "$status" in
      accepted) break ;;
      wrong_answer|compile_error|runtime_error|time_limit_exceeded|internal_error) printf 'unexpected result: %s\n' "$response" >&2; exit 1 ;;
    esac
    sleep 1
  done
  test "$status" = accepted
  attempts="$("${compose[@]}" exec -T postgres psql -U postgres -d cj_judge -Atc "SELECT judge_attempts FROM submissions WHERE id='$id'")"
  test "$attempts" -ge 2
  printf 'in-flight %s recovered submission=%s attempts=%s final=accepted trace=%s\n' "$mode" "$id" "$attempts" "$trace_id"
}

verify_recovery SIGKILL
verify_recovery forced-drain
