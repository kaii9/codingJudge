#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd "$(dirname "$0")/.." && pwd)"
compose=(docker compose -f "$task_root/deploy/compose/microservices.yml")
api="${CJ_MICRO_API_URL:-http://127.0.0.1:28080}"
cookie="$(mktemp /tmp/gojudge-micro-verify.XXXXXX)"
user="micro_$(date +%s)_$$"
cleanup() {
  rm -f "$cookie"
  "${compose[@]}" start executor >/dev/null 2>&1 || true
}
trap cleanup EXIT
register="$(jq -nc --arg username "$user" '{username:$username,password:"microservices-test-password"}')"
curl -fsS -c "$cookie" -H 'Content-Type: application/json' -d "$register" "$api/auth/register" >/dev/null
submit() {
  local language="$1" code="$2" sequence="$3" response id status
  response="$(curl -fsS -b "$cookie" -H 'Content-Type: application/json' -H "Idempotency-Key: $user-$sequence" \
    -d "$(jq -nc --arg language "$language" --arg code "$code" '{problemId:"sum",language:$language,code:$code}')" "$api/submissions")"
  id="$(jq -r '.id' <<<"$response")"
  for _ in $(seq 1 90); do
    response="$(curl -fsS -b "$cookie" "$api/submissions/$id")"
    status="$(jq -r '.status' <<<"$response")"
    case "$status" in
      accepted) printf 'accepted language=%s submission=%s\n' "$language" "$id"; return 0 ;;
      wrong_answer|compile_error|runtime_error|time_limit_exceeded|internal_error) printf 'unexpected result: %s\n' "$response" >&2; return 1 ;;
    esac
    sleep 1
  done
  printf 'submission timed out: %s\n' "$id" >&2; return 1
}
python_code='a,b=map(int,input().split());print(a+b)'
go_code='package main
import "fmt"
func main(){var a,b int;fmt.Scan(&a,&b);fmt.Println(a+b)}'
cpp_code='#include <iostream>
int main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<std::endl;}'
submit python "$python_code" python
submit go "$go_code" go
submit cpp "$cpp_code" cpp

for _ in $(seq 1 30); do
  leaderboard="$(curl -fsS "$api/leaderboard")"
  if jq -e --arg user "$user" '.[]|select(.username==$user and .solved==1 and .acceptedSubmissions==3)' <<<"$leaderboard" >/dev/null; then break; fi
  sleep 1
done
jq -e --arg user "$user" '.[]|select(.username==$user and .solved==1 and .acceptedSubmissions==3)' <<<"$leaderboard" >/dev/null
printf 'leaderboard projection converged\n'

platform_id="$("${compose[@]}" ps -q platform)"
judge_id="$("${compose[@]}" ps -q judge-api)"
worker_id="$("${compose[@]}" ps -q worker)"
"${compose[@]}" stop executor >/dev/null
submit python "$python_code" failover
"${compose[@]}" start executor >/dev/null
printf 'single executor outage recovered through healthy executor\n'

"${compose[@]}" up -d --no-deps --force-recreate executor >/dev/null
submit python "$python_code" rolling
test "$platform_id" = "$("${compose[@]}" ps -q platform)"
test "$judge_id" = "$("${compose[@]}" ps -q judge-api)"
test "$worker_id" = "$("${compose[@]}" ps -q worker)"
printf 'executor recreated independently; platform/judge/worker container IDs unchanged\n'

history="$(curl -fsS -b "$cookie" "$api/submissions")"
test "$(jq length <<<"$history")" -eq 5
id="$(jq -r '.[0].id' <<<"$history")"
artifacts="$(curl -fsS -b "$cookie" "$api/submissions/$id/artifacts")"
artifact_id="$(jq -r '.[]|select(.kind=="source")|.id' <<<"$artifacts" | head -1)"
test -n "$artifact_id"
source="$(curl -fsS -b "$cookie" "$api/submissions/$id/artifacts/$artifact_id")"
test "$source" = "$python_code"
printf 'history and remote artifact download passed\n'

for _ in $(seq 1 30); do
  pending="$("${compose[@]}" exec -T redis redis-cli XPENDING judge:submissions judge-workers | head -1 | tr -d '\r')"
  results_pending="$("${compose[@]}" exec -T redis redis-cli XPENDING judge:results platform-projectors | head -1 | tr -d '\r')"
  test "$pending" = 0 && test "$results_pending" = 0 && break
  sleep 1
done
test "$pending" = 0
test "$results_pending" = 0
printf 'judge and result streams drained; verification passed\n'
