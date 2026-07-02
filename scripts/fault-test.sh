#!/usr/bin/env bash
set -euo pipefail

api_url="${API_URL:-http://localhost:18080}"
lease_wait="${LEASE_WAIT_SECONDS:-35}"

docker compose up -d --build --scale worker=2
workers=($(docker compose ps -q worker))
if [[ "${#workers[@]}" -lt 2 ]]; then
  echo "two worker containers were not created" >&2
  exit 1
fi
original_worker="${workers[0]}"
standby_worker="${workers[1]}"
cleanup() {
  if [[ -n "${original_worker:-}" ]]; then
    docker unpause "$original_worker" >/dev/null 2>&1 || true
  fi
  if [[ -n "${standby_worker:-}" ]]; then
    docker start "$standby_worker" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
docker stop "$standby_worker" >/dev/null

payload='{"problemId":"sum","language":"python","code":"while True:\n    pass"}'
submission_id="$(curl --fail --silent --show-error -X POST "$api_url/submissions" \
  -H 'Content-Type: application/json' -d "$payload" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [[ -z "$submission_id" ]]; then
  echo "submission id was not returned" >&2
  exit 1
fi

for _ in $(seq 1 200); do
  status="$(curl --fail --silent "$api_url/submissions/$submission_id" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')"
  if [[ "$status" == "running" ]]; then
    break
  fi
  sleep 0.05
done
if [[ "${status:-}" != "running" ]]; then
  echo "submission never entered running state" >&2
  exit 1
fi

docker pause "$original_worker" >/dev/null
docker start "$standby_worker" >/dev/null
sleep "$lease_wait"
docker rm -f "$original_worker" >/dev/null
original_worker=""

for _ in $(seq 1 120); do
  status="$(curl --fail --silent "$api_url/submissions/$submission_id" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')"
  case "$status" in
    accepted|wrong_answer|runtime_error|time_limit_exceeded|internal_error)
      break
      ;;
  esac
  sleep 0.5
done

case "${status:-}" in
  accepted|wrong_answer|runtime_error|time_limit_exceeded|internal_error) ;;
  *) echo "submission did not recover: ${status:-unknown}" >&2; exit 1 ;;
esac

pending="$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers | head -n 1 | tr -d '\r')"
if [[ "$pending" != "0" ]]; then
  echo "unexpected pending jobs: $pending" >&2
  exit 1
fi

echo "recovered $submission_id with status $status"
