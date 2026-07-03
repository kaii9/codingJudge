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
error() { echo -e "${RED}[error]${NC} $*"; }

# Restore to 2 workers on exit.
cleanup() {
  info "restoring 2 workers..."
  docker compose up -d --scale worker=2 --wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

info "building images..."
docker compose build 2>&1

info "recording machine metadata..."
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "git_commit: $(git -C "$ROOT" rev-parse --short HEAD)"
  echo "os: $(uname -s)"
  echo "arch: $(uname -m)"
  echo "logical_cpus: $(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo unknown)"
} > "$RESULTS/meta.txt"

SCENARIO="${K6_SCENARIO:-submissions.js}"
K6_ARGS="${K6_ARGS:---env K6_RATE=5 --env K6_VUS=10 --env K6_DURATION=2m}"

for workers in 1 2 4; do
  info "=== scaling to $workers worker(s) ==="
  docker compose up -d --scale worker=$workers --wait 2>&1

  # Wait for Prometheus discovery.
  info "waiting for $workers healthy worker target(s)..."
  for i in $(seq 1 30); do
    count=$(docker compose exec -T prometheus wget -qO- 'http://localhost:9090/api/v1/query?query=count(up{job="codingjudge-worker"}==1)' 2>/dev/null | jq -r '.data.result[0].value[1]' 2>/dev/null || echo 0)
    if [ "$count" = "$workers" ]; then
      break
    fi
    sleep 2
  done

  # Capture peak pending script (samples every 5s during run).
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
  docker compose --profile loadtest run --rm k6 k6 run "/scripts/$SCENARIO" $K6_ARGS --summary-export="/results/k6-worker-${workers}.json" 2>&1 || error "k6 returned non-zero (threshold failures)"

  kill $CAPTURE_PID 2>/dev/null || true
  wait $CAPTURE_PID 2>/dev/null || true

  # Extract peak pending.
  PEAK=$(awk -F',' 'NR>1 {if($2+0>max) max=$2+0} END{print max+0}' "$PENDING_LOG")
  echo "peak_pending_w${workers}: $PEAK" >> "$RESULTS/meta.txt"

  info "waiting for Pending=0..."
  for i in $(seq 1 60); do
    pending=$(docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers 2>/dev/null | awk '{print $1}' || echo 0)
    if [ "${pending:-0}" = "0" ]; then
      break
    fi
    sleep 2
  done
done

info "rendering report..."
go run "$BENCH_DIR/render-benchmark-report.go" "$RESULTS/meta.txt" \
  "$RESULTS/k6-worker-1.json" \
  "$RESULTS/k6-worker-2.json" \
  "$RESULTS/k6-worker-4.json" > "$REPORT"

info "benchmark complete. Report: $REPORT"
