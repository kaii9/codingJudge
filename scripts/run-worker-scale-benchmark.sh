#!/usr/bin/env bash
set -euo pipefail

BENCH_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$BENCH_DIR")"
RESULTS="$ROOT/loadtest/results"
REPORT="$ROOT/docs/benchmarks/2026-07-03-worker-scaling.md"
REPORT_TMP="${REPORT}.tmp"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

info()  { echo -e "${GREEN}[info]${NC} $*"; }
die()   { echo -e "${RED}[fatal]${NC} $*"; exit 1; }
warn()  { echo -e "${RED}[warn]${NC} $*"; }

cleanup() {
  local code=$?
  info "restoring 2 workers..."
  docker compose up -d --scale worker=2 --wait 2>/dev/null || true
  if [ $code -ne 0 ]; then
    warn "benchmark failed with exit code $code"
  fi
  exit $code
}
trap cleanup EXIT INT TERM

mkdir -p "$RESULTS"
mkdir -p "$(dirname "$REPORT")"

info "building images..."
docker compose build

DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)
MEMORY=$(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%.0f GB", $1/1024/1024/1024}' || echo unknown)
K6_VERSION=$(docker compose --profile loadtest run --rm --entrypoint k6 k6 version 2>/dev/null | head -1 | sed 's/.*k6 v//' | awk '{print $1}' || echo unknown)
JUDGE_IMAGES="golang:1.25-alpine, python:3.12-alpine, gcc:13"

info "recording machine metadata..."
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "git_commit: $(git -C "$ROOT" rev-parse --short HEAD)"
  echo "os: $(uname -s)"
  echo "arch: $(uname -m)"
  echo "logical_cpus: $(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo unknown)"
  echo "memory: ${MEMORY}"
  echo "docker_version: ${DOCKER_VERSION}"
  echo "k6_version: ${K6_VERSION}"
  echo "judge_images: ${JUDGE_IMAGES}"
} > "$RESULTS/meta.txt"

REQUIRED_METRICS="http_reqs http_req_duration http_req_failed submissions_created submissions_accepted iterations"

SCENARIO="${K6_SCENARIO:-submissions.js}"
CJ_RATE="${CJ_RATE:-1}"
CJ_PREALLOCATED_VUS="${CJ_PREALLOCATED_VUS:-20}"
CJ_MAX_VUS="${CJ_MAX_VUS:-30}"
CJ_DURATION="${CJ_DURATION:-2m}"
CJ_TIMEOUT="${CJ_TIMEOUT:-120}"

# Use non-reserved k6 env var names so the constant-arrival-rate scenario is not overridden.
K6_ARGS="--env CJ_RATE=$CJ_RATE --env CJ_PREALLOCATED_VUS=$CJ_PREALLOCATED_VUS --env CJ_MAX_VUS=$CJ_MAX_VUS --env CJ_DURATION=$CJ_DURATION --env CJ_JUDGE_TIMEOUT_SECONDS=$CJ_TIMEOUT"

{
  echo "offered_rate: ${CJ_RATE} req/s"
  echo "duration: ${CJ_DURATION}"
  echo "preallocated_vus: ${CJ_PREALLOCATED_VUS}"
  echo "max_vus: ${CJ_MAX_VUS}"
  echo "judge_timeout_seconds: ${CJ_TIMEOUT}"
  echo "scenario: ${SCENARIO}"
  echo "worker_concurrency: 1"
} >> "$RESULTS/meta.txt"

# Pre-compute expected submission count range.
# rate * duration_in_seconds * 1.3 → upper bound, 0.5 → lower bound
DURATION_SECS=$(echo "$CJ_DURATION" | sed 's/m$/*60/' | bc)
EXPECTED_MIN=$(echo "$CJ_RATE * $DURATION_SECS * 0.5 / 1" | bc 2>/dev/null || echo 30)
EXPECTED_MAX=$(echo "$CJ_RATE * $DURATION_SECS * 1.5 / 1" | bc 2>/dev/null || echo 360)

validate_summary() {
  local summary="$1" label="$2"
  [ ! -s "$summary" ] && die "$label: summary file missing or empty: $summary"
  for metric in $REQUIRED_METRICS; do
    jq -e ".metrics[\"$metric\"]" "$summary" >/dev/null 2>&1 || die "$label: required metric '$metric' missing from $summary"
  done
  local dropped=$(jq -r '.metrics.dropped_iterations.count // 0' "$summary" 2>/dev/null)
  if [ "${dropped:-0}" != "0" ] && [ "${dropped:-0}" != "null" ]; then
    die "$label: dropped iterations = $dropped, must be 0"
  fi
  local created=$(jq -r '.metrics.submissions_created.count // 0' "$summary" 2>/dev/null)
  local iterations=$(jq -r '.metrics.iterations.count // 0' "$summary" 2>/dev/null)
  local accepted=$(jq -r '.metrics.submissions_accepted.count // 0' "$summary" 2>/dev/null)
  if [ "$created" != "$iterations" ]; then
    die "$label: submissions_created ($created) != iterations ($iterations)"
  fi
  local min_accepted=$(echo "$created * 0.95 / 1" | bc 2>/dev/null || echo 0)
  if [ "$accepted" -lt "$min_accepted" ]; then
    die "$label: accepted ($accepted) < 95% of created ($created)"
  fi
  if [ "$created" -lt "$EXPECTED_MIN" ] || [ "$created" -gt "$EXPECTED_MAX" ]; then
    die "$label: created=$created outside expected range [$EXPECTED_MIN, $EXPECTED_MAX]"
  fi
  info "  summary valid: $label (created=$created, accepted=$accepted, iterations=$iterations, dropped=0)"
}

for workers in 1 2 4; do
  PRE_PENDING=$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers 2>/dev/null | awk '{print $1}' || echo -1)
  if [ "${PRE_PENDING:-0}" != "0" ]; then
    die "pre-round Redis Pending is $PRE_PENDING, expected 0"
  fi

  info "=== scaling to $workers worker(s) ==="
  docker compose up -d --scale worker=$workers --wait

  info "waiting for $workers healthy worker target(s)..."
  found=0
  for i in $(seq 1 30); do
    count=$(docker compose exec -T prometheus wget -qO- 'http://localhost:9090/api/v1/query?query=count(up{job="codingjudge-worker"}==1)' 2>/dev/null | jq -r '.data.result[0].value[1]' 2>/dev/null || echo 0)
    if [ "$count" = "$workers" ]; then
      found=1
      break
    fi
    sleep 2
  done
  if [ "$found" -ne 1 ]; then
    die "worker target count did not reach $workers within 60s"
  fi
  info "  $workers worker target(s) confirmed"

  info "starting pending capture..."
  PENDING_LOG="$RESULTS/pending-w${workers}.csv"
  echo "ts,count" > "$PENDING_LOG"
  (
    while true; do
      cnt=$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers 2>/dev/null | awk '{print $1}' || echo 0)
      echo "$(date +%s),${cnt:-0}" >> "$PENDING_LOG"
      sleep 5
    done
  ) &
  CAPTURE_PID=$!

  info "running k6 scenario: $SCENARIO"
  SUMMARY="$RESULTS/k6-worker-${workers}.json"
  set +e
  docker compose --profile loadtest run --rm k6 k6 run "/scripts/$SCENARIO" $K6_ARGS --summary-export="/results/k6-worker-${workers}.json" 2>&1
  k6_exit=$?
  set -e

  kill $CAPTURE_PID 2>/dev/null || true
  wait $CAPTURE_PID 2>/dev/null || true

  if [ $k6_exit -ne 0 ]; then
    die "k6 returned non-zero (threshold failures), exit=$k6_exit"
  fi

  validate_summary "$SUMMARY" "worker-$workers"

  PEAK=$(awk -F',' 'NR>1 {if($2+0>max) max=$2+0} END{print max+0}' "$PENDING_LOG")
  echo "peak_pending_w${workers}: $PEAK" >> "$RESULTS/meta.txt"

  info "waiting for Pending=0..."
  drained=0
  for i in $(seq 1 60); do
    pending=$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers 2>/dev/null | awk '{print $1}' || echo 0)
    if [ "${pending:-0}" = "0" ]; then
      drained=1
      break
    fi
    sleep 2
  done
  if [ "$drained" -ne 1 ]; then
    die "Redis Pending did not reach 0 within 120s (got ${pending:-?})"
  fi
  info "  Pending drained to 0"
done

info "final Redis Pending check..."
FINAL_PENDING=$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers 2>/dev/null | awk '{print $1}' || echo -1)
if [ "${FINAL_PENDING:-0}" != "0" ]; then
  die "final Redis Pending is $FINAL_PENDING, expected 0"
fi
info "  Redis Pending = 0 (confirmed)"

info "rendering report..."
for i in 1 2 4; do
  SUMMARY="$RESULTS/k6-worker-${i}.json"
  if [ ! -s "$SUMMARY" ]; then
    die "summary file missing before render: $SUMMARY"
  fi
done
rm -f "$REPORT_TMP"
go run "$BENCH_DIR/render-benchmark-report.go" "$RESULTS/meta.txt" \
  "$RESULTS/k6-worker-1.json" \
  "$RESULTS/k6-worker-2.json" \
  "$RESULTS/k6-worker-4.json" > "$REPORT_TMP"

if [ ! -s "$REPORT_TMP" ]; then
  die "report file is empty"
fi
mv "$REPORT_TMP" "$REPORT"

info "benchmark complete. Report: $REPORT"
