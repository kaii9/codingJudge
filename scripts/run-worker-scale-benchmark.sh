#!/usr/bin/env bash
set -euo pipefail

BENCH_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$BENCH_DIR")"
RESULTS="$ROOT/loadtest/results"
REPORT="$ROOT/docs/benchmarks/2026-07-03-worker-scaling.md"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

info()  { echo -e "${GREEN}[info]${NC} $*"; }
die()   { echo -e "${RED}[fatal]${NC} $*"; exit 1; }
warn()  { echo -e "${RED}[warn]${NC} $*"; }

EXIT_CODE=0

# Restore to 2 workers on exit, preserving the original exit code.
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

info "building images..."
docker compose build

info "recording machine metadata..."
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "git_commit: $(git -C "$ROOT" rev-parse --short HEAD)"
  echo "os: $(uname -s)"
  echo "arch: $(uname -m)"
  echo "logical_cpus: $(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo unknown)"
} > "$RESULTS/meta.txt"

REQUIRED_METRICS="http_reqs http_req_duration http_req_failed"

SCENARIO="${K6_SCENARIO:-submissions.js}"
K6_ARGS="${K6_ARGS:---env K6_RATE=1 --env K6_VUS=4 --env K6_DURATION=2m --env JUDGE_TIMEOUT_SECONDS=60}"

for workers in 1 2 4; do
  info "=== scaling to $workers worker(s) ==="
  docker compose up -d --scale worker=$workers --wait

  # Wait for Prometheus discovery — must reach target count or die.
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

  # Capture peak pending (samples every 5s during run).
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

  # Validate summary file exists and contains required metrics.
  if [ ! -s "$SUMMARY" ]; then
    die "k6 summary file missing or empty: $SUMMARY"
  fi
  for metric in $REQUIRED_METRICS; do
    if ! jq -e ".metrics[\"$metric\"]" "$SUMMARY" >/dev/null 2>&1; then
      die "required metric '$metric' missing from $SUMMARY"
    fi
  done
  info "  summary file validated: $SUMMARY"

  # Extract peak pending.
  PEAK=$(awk -F',' 'NR>1 {if($2+0>max) max=$2+0} END{print max+0}' "$PENDING_LOG")
  echo "peak_pending_w${workers}: $PEAK" >> "$RESULTS/meta.txt"

  # Wait for Pending=0 — must reach zero or die.
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

# Final Redis Pending confirmation.
info "final Redis Pending check..."
FINAL_PENDING=$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers 2>/dev/null | awk '{print $1}' || echo -1)
if [ "${FINAL_PENDING:-0}" != "0" ]; then
  die "final Redis Pending is $FINAL_PENDING, expected 0"
fi
info "  Redis Pending = 0 (confirmed)"

info "rendering report..."
mkdir -p "$(dirname "$REPORT")"
for i in 1 2 4; do
  SUMMARY="$RESULTS/k6-worker-${i}.json"
  if [ ! -s "$SUMMARY" ]; then
    die "summary file missing before render: $SUMMARY"
  fi
done
go run "$BENCH_DIR/render-benchmark-report.go" "$RESULTS/meta.txt" \
  "$RESULTS/k6-worker-1.json" \
  "$RESULTS/k6-worker-2.json" \
  "$RESULTS/k6-worker-4.json" > "$REPORT"

if [ ! -s "$REPORT" ]; then
  die "report file is empty"
fi

info "benchmark complete. Report: $REPORT"
