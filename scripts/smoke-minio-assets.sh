#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

API_URL="${API_URL:-http://localhost:18080}"
DB_DSN="${TEST_DATABASE_URL:-postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable}"
COOKIE_FILE="$(mktemp)"
USERNAME="asset_smoke_$(date +%s)"

cleanup() {
  rm -f "$COOKIE_FILE"
  if [[ "${SMOKE_KEEP_STACK:-0}" != "1" ]]; then
    docker compose stop api worker postgres redis minio >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker compose up -d --build postgres redis minio api worker
docker compose --profile assets build case-uploader
docker compose --profile assets run --rm case-uploader

curl -fsS -c "$COOKIE_FILE" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USERNAME\",\"password\":\"correct-password\"}" \
  "$API_URL/auth/register" >/dev/null

submission_id="$(curl -fsS -b "$COOKIE_FILE" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: asset-$USERNAME" \
  -d '{"problemId":"sum","language":"python","code":"a,b=map(int,input().split())\nprint(a+b)"}' \
  "$API_URL/submissions" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"

if [[ -z "$submission_id" ]]; then
  echo "submission id was not returned" >&2
  exit 1
fi

for _ in $(seq 1 60); do
  body="$(curl -fsS -b "$COOKIE_FILE" "$API_URL/submissions/$submission_id")"
  status="$(printf '%s' "$body" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')"
  if [[ "$status" == "accepted" ]]; then
    break
  fi
  if [[ "$status" == "wrong_answer" || "$status" == "runtime_error" || "$status" == "time_limit_exceeded" || "$status" == "internal_error" ]]; then
    echo "submission finished with unexpected status: $status" >&2
    echo "$body" >&2
    exit 1
  fi
  sleep 1
done

if [[ "${status:-}" != "accepted" ]]; then
  echo "submission did not reach accepted, last status=${status:-unknown}" >&2
  exit 1
fi

artifact_count="$(docker compose exec -T postgres psql -U codingjudge -d codingjudge -tAc "SELECT count(*) FROM submission_artifacts WHERE submission_id='$submission_id'")"
if [[ "$artifact_count" -lt 1 ]]; then
  echo "expected at least one submission artifact, got $artifact_count" >&2
  exit 1
fi

object_key="$(docker compose exec -T postgres psql -U codingjudge -d codingjudge -tAc "SELECT object_key FROM submission_artifacts WHERE submission_id='$submission_id' AND kind='source' LIMIT 1")"
if [[ -z "$object_key" ]]; then
  echo "source artifact object key was not stored" >&2
  exit 1
fi

docker compose exec -T minio sh -lc "mc alias set cj http://localhost:9000 minioadmin minioadmin >/dev/null && mc stat 'cj/codingjudge-assets/$object_key' >/dev/null"

case_count="$(docker compose exec -T postgres psql -U codingjudge -d codingjudge -tAc "SELECT count(*) FROM problem_test_cases WHERE problem_id='sum' AND input_object_key IS NOT NULL AND expected_output_object_key IS NOT NULL")"
if [[ "$case_count" -lt 2 ]]; then
  echo "expected object-backed sum cases, got $case_count" >&2
  exit 1
fi

hot20_case_count="$(docker compose exec -T postgres psql -U codingjudge -d codingjudge -tAc "SELECT count(*) FROM problem_test_cases tc JOIN problems p ON p.id = tc.problem_id WHERE p.collection='hot20' AND tc.input_object_key IS NOT NULL AND tc.expected_output_object_key IS NOT NULL")"
if [[ "$hot20_case_count" -lt 120 ]]; then
  echo "expected Hot20 object-backed cases, got $hot20_case_count" >&2
  exit 1
fi

echo "minio asset smoke passed submission=$submission_id artifacts=$artifact_count cases=$case_count hot20_cases=$hot20_case_count"
