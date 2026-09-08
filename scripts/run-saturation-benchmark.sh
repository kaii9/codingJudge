#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
RESULTS="$ROOT/loadtest/results"
REPORT="$ROOT/docs/benchmarks/2026-09-08-saturation-scaling.md"
REPORT_TMP="${REPORT}.tmp"
CSV="$RESULTS/saturation.csv"
META="$RESULTS/saturation-meta.txt"

SAT_BATCH_SIZE="${SAT_BATCH_SIZE:-60}"
SAT_VUS="${SAT_VUS:-20}"
SAT_MAX_DURATION="${SAT_MAX_DURATION:-30s}"
SAT_DRAIN_TIMEOUT_SECONDS="${SAT_DRAIN_TIMEOUT_SECONDS:-180}"
BENCH_RATE_LIMIT="${BENCH_RATE_LIMIT:-100000}"
BENCH_RATE_BURST="${BENCH_RATE_BURST:-1000}"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

CAPTURE_PID=""

info() { echo -e "${GREEN}[info]${NC} $*"; }
warn() { echo -e "${RED}[warn]${NC} $*"; }
die() { echo -e "${RED}[fatal]${NC} $*"; exit 1; }

stop_capture() {
  if [ -n "$CAPTURE_PID" ]; then
    kill "$CAPTURE_PID" 2>/dev/null || true
    wait "$CAPTURE_PID" 2>/dev/null || true
    CAPTURE_PID=""
  fi
}

cleanup() {
  local code=$?
  trap - EXIT INT TERM
  stop_capture
  info "restoring the default API admission limit and two workers..."
  docker compose up -d --scale worker=2 --wait >/dev/null 2>&1 || true
  if [ "$code" -ne 0 ]; then
    warn "benchmark failed with exit code $code"
  fi
  exit "$code"
}
trap cleanup EXIT INT TERM

case "$SAT_BATCH_SIZE" in ''|*[!0-9]*) die "SAT_BATCH_SIZE must be a positive integer" ;; esac
case "$SAT_VUS" in ''|*[!0-9]*) die "SAT_VUS must be a positive integer" ;; esac
case "$SAT_DRAIN_TIMEOUT_SECONDS" in ''|*[!0-9]*) die "SAT_DRAIN_TIMEOUT_SECONDS must be a positive integer" ;; esac
[ "$SAT_BATCH_SIZE" -gt 0 ] || die "SAT_BATCH_SIZE must be greater than zero"
[ "$SAT_VUS" -gt 0 ] || die "SAT_VUS must be greater than zero"
[ "$SAT_DRAIN_TIMEOUT_SECONDS" -gt 0 ] || die "SAT_DRAIN_TIMEOUT_SECONDS must be greater than zero"
command -v docker >/dev/null || die "docker is required"
command -v jq >/dev/null || die "jq is required"

queue_stats() {
  docker compose exec -T redis redis-cli --raw XINFO GROUPS judge:submissions 2>/dev/null | awk '
    $0 == "name" {
      getline
      target = ($0 == "judge-workers")
      next
    }
    target && $0 == "pending" {
      getline
      pending = $0
      next
    }
    target && $0 == "lag" {
      getline
      lag = $0
      next
    }
    END {
      if (pending == "") pending = 0
      if (lag == "" || lag == "nil") lag = 0
      print pending + 0, lag + 0
    }
  '
}

query_run() {
  local run_id="$1"
  docker compose exec -T postgres psql -U codingjudge -d codingjudge -tA -F ',' -v ON_ERROR_STOP=1 -c "
    SELECT COUNT(*),
           COUNT(*) FILTER (WHERE s.status = 'accepted'),
           COUNT(*) FILTER (WHERE s.status IN (
             'accepted', 'wrong_answer', 'compile_error', 'runtime_error',
             'time_limit_exceeded', 'internal_error'
           )),
           COALESCE(EXTRACT(EPOCH FROM (MAX(s.updated_at) - MIN(s.created_at))), 0)
      FROM submissions s
      JOIN users u ON u.id = s.user_id
     WHERE LEFT(u.username, LENGTH('k6_${run_id}_')) = 'k6_${run_id}_';"
}

validate_summary() {
  local summary="$1" label="$2"
  [ -s "$summary" ] || die "$label: k6 summary is missing or empty"
  for metric in iterations submissions_created http_req_failed logical_failures http_req_duration; do
    jq -e ".metrics[\"$metric\"]" "$summary" >/dev/null \
      || die "$label: required metric '$metric' is missing"
  done

  local iterations created
  iterations=$(jq -r '.metrics.iterations.count // 0' "$summary")
  created=$(jq -r '.metrics.submissions_created.count // 0' "$summary")
  [ "$iterations" = "$SAT_BATCH_SIZE" ] \
    || die "$label: iterations=$iterations, expected $SAT_BATCH_SIZE"
  [ "$created" = "$SAT_BATCH_SIZE" ] \
    || die "$label: submissions_created=$created, expected $SAT_BATCH_SIZE"
  jq -e '.metrics.http_req_failed.value == 0' "$summary" >/dev/null \
    || die "$label: HTTP failures are non-zero"
  jq -e '.metrics.logical_failures.value == 0' "$summary" >/dev/null \
    || die "$label: logical failures are non-zero"
}

mkdir -p "$RESULTS" "$(dirname "$REPORT")"

info "building backend and judge images..."
docker compose build api worker migrate judge-images

DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)
MEMORY=$(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%.0f GB", $1/1024/1024/1024}' || echo unknown)
TRACKED_TREE="dirty"
if git -C "$ROOT" diff --quiet && git -C "$ROOT" diff --cached --quiet; then
  TRACKED_TREE="clean"
fi

{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "git_commit: $(git -C "$ROOT" rev-parse --short HEAD)"
  echo "git_tracked_tree: $TRACKED_TREE"
  echo "os: $(uname -s)"
  echo "arch: $(uname -m)"
  echo "logical_cpus: $(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo unknown)"
  echo "memory: $MEMORY"
  echo "docker_version: $DOCKER_VERSION"
  echo "batch_size: $SAT_BATCH_SIZE"
  echo "vus: $SAT_VUS"
  echo "language: python"
  echo "problem_id: echo"
  echo "worker_concurrency: 1"
  echo "repetitions: 1"
} > "$META"

echo "workers,batch,accepted,makespan_seconds,throughput_per_second,http_p95_ms,peak_pending,peak_lag,peak_outstanding" > "$CSV"

for workers in 1 2 4; do
  info "scaling to $workers worker(s) with benchmark admission limits..."
  SUBMISSION_RATE_LIMIT_PER_MINUTE="$BENCH_RATE_LIMIT" \
  SUBMISSION_RATE_LIMIT_BURST="$BENCH_RATE_BURST" \
    docker compose up -d --scale worker="$workers" --wait

  read -r pre_pending pre_lag < <(queue_stats)
  if [ "$pre_pending" -ne 0 ] || [ "$pre_lag" -ne 0 ]; then
    die "worker-$workers: queue is not empty before the round (pending=$pre_pending, lag=$pre_lag)"
  fi

  run_id="satw${workers}$(date +%s)"
  summary="$RESULTS/saturation-w${workers}.json"
  log="$RESULTS/saturation-w${workers}.log"
  queue_log="$RESULTS/saturation-queue-w${workers}.csv"
  rm -f "$summary" "$log"
  echo "timestamp,pending,lag,outstanding" > "$queue_log"

  info "capturing Redis Stream backlog for run $run_id..."
  (
    while true; do
      read -r pending lag < <(queue_stats)
      echo "$(date +%s),$pending,$lag,$((pending + lag))" >> "$queue_log"
      sleep 1
    done
  ) &
  CAPTURE_PID=$!

  info "submitting a burst of $SAT_BATCH_SIZE Python jobs with $SAT_VUS VUs..."
  set +e
  SUBMISSION_RATE_LIMIT_PER_MINUTE="$BENCH_RATE_LIMIT" \
  SUBMISSION_RATE_LIMIT_BURST="$BENCH_RATE_BURST" \
  docker compose --profile loadtest run --rm k6 \
    k6 run /scripts/saturation.js \
    --env "CJ_RUN_ID=$run_id" \
    --env "CJ_ITERATIONS=$SAT_BATCH_SIZE" \
    --env "CJ_VUS=$SAT_VUS" \
    --env "CJ_MAX_DURATION=$SAT_MAX_DURATION" \
    --summary-export="/results/saturation-w${workers}.json" 2>&1 | tee "$log"
  k6_exit=${PIPESTATUS[0]}
  set -e
  [ "$k6_exit" -eq 0 ] || die "worker-$workers: k6 exited with $k6_exit"
  validate_summary "$summary" "worker-$workers"

  info "waiting for all jobs in run $run_id to reach a terminal state..."
  terminal=0
  for _ in $(seq 1 "$SAT_DRAIN_TIMEOUT_SECONDS"); do
    IFS=',' read -r total accepted terminal makespan < <(query_run "$run_id")
    if [ "$total" -eq "$SAT_BATCH_SIZE" ] && [ "$terminal" -eq "$SAT_BATCH_SIZE" ]; then
      break
    fi
    sleep 1
  done
  if [ "$total" -ne "$SAT_BATCH_SIZE" ] || [ "$terminal" -ne "$SAT_BATCH_SIZE" ]; then
    die "worker-$workers: drain timed out (total=$total, terminal=$terminal, expected=$SAT_BATCH_SIZE)"
  fi
  [ "$accepted" -eq "$SAT_BATCH_SIZE" ] \
    || die "worker-$workers: accepted=$accepted, expected $SAT_BATCH_SIZE"

  drained=0
  for _ in $(seq 1 20); do
    read -r pending lag < <(queue_stats)
    if [ "$pending" -eq 0 ] && [ "$lag" -eq 0 ]; then
      drained=1
      break
    fi
    sleep 1
  done
  [ "$drained" -eq 1 ] \
    || die "worker-$workers: database is terminal but queue did not drain (pending=$pending, lag=$lag)"

  stop_capture
  peak_pending=$(awk -F',' 'NR > 1 && $2 + 0 > max {max = $2 + 0} END {print max + 0}' "$queue_log")
  peak_lag=$(awk -F',' 'NR > 1 && $3 + 0 > max {max = $3 + 0} END {print max + 0}' "$queue_log")
  peak_outstanding=$(awk -F',' 'NR > 1 && $4 + 0 > max {max = $4 + 0} END {print max + 0}' "$queue_log")
  [ "$peak_lag" -gt 0 ] \
    || die "worker-$workers: no Stream lag was observed; increase SAT_BATCH_SIZE"
  awk -v value="$makespan" 'BEGIN {exit !(value > 0)}' \
    || die "worker-$workers: invalid makespan $makespan"

  throughput=$(awk -v batch="$SAT_BATCH_SIZE" -v seconds="$makespan" 'BEGIN {printf "%.6f", batch / seconds}')
  http_p95=$(jq -r '.metrics.http_req_duration["p(95)"] // 0' "$summary")
  echo "$workers,$total,$accepted,$makespan,$throughput,$http_p95,$peak_pending,$peak_lag,$peak_outstanding" >> "$CSV"
  info "worker-$workers complete: ${throughput}/s, makespan=${makespan}s, peak backlog=$peak_outstanding"
done

info "rendering the validated benchmark report..."
rm -f "$REPORT_TMP"
(
  cd "$ROOT"
  go run ./scripts/saturation-report "$META" "$CSV"
) > "$REPORT_TMP"
[ -s "$REPORT_TMP" ] || die "generated report is empty"
mv "$REPORT_TMP" "$REPORT"

info "saturation benchmark complete: $REPORT"
