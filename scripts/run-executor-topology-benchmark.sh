#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
RESULTS="$ROOT/loadtest/results"
REPORT="${ET_REPORT_PATH:-$ROOT/docs/benchmarks/2026-09-09-executor-topology.md}"
REPORT_TMP="${REPORT}.tmp"
CSV="$RESULTS/executor-topology.csv"
META="$RESULTS/executor-topology-meta.txt"
PROJECT="${ET_COMPOSE_PROJECT:-codingjudge}"

ET_BATCH_SIZE="${ET_BATCH_SIZE:-60}"
ET_VUS="${ET_VUS:-20}"
ET_MAX_DURATION="${ET_MAX_DURATION:-30s}"
ET_DRAIN_TIMEOUT_SECONDS="${ET_DRAIN_TIMEOUT_SECONDS:-180}"
ET_REPETITIONS="${ET_REPETITIONS:-3}"
ET_WORKERS="${ET_WORKERS:-4}"
ET_PROBLEM_TIME_LIMIT_MS="${ET_PROBLEM_TIME_LIMIT_MS:-10000}"
BENCH_RATE_LIMIT="${BENCH_RATE_LIMIT:-100000}"
BENCH_RATE_BURST="${BENCH_RATE_BURST:-1000}"

BASE_COMPOSE=(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
RESTORE_COMPOSE=("${BASE_COMPOSE[@]}" -f "$ROOT/deploy/compose/multinode.yml")
COMPOSE=()
EXECUTOR_SERVICES=()
EXPECTED_EXECUTORS=0
EXPECTED_DAEMONS=0
EXPECTED_SLOTS=0
CAPTURE_PID=""
ORIGINAL_PROBLEM_TIME_LIMIT=""
PROBLEM_LIMIT_CHANGED=0

info() { printf '[info] %s\n' "$*"; }
warn() { printf '[warn] %s\n' "$*" >&2; }
die() { printf '[fatal] %s\n' "$*" >&2; exit 1; }

stop_capture() {
  if [ -n "$CAPTURE_PID" ]; then
    kill "$CAPTURE_PID" 2>/dev/null || true
    wait "$CAPTURE_PID" 2>/dev/null || true
    CAPTURE_PID=""
  fi
}

unpause_project_containers() {
  docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" | while IFS= read -r container_id; do
    [ -n "$container_id" ] || continue
    if [ "$(docker inspect "$container_id" --format '{{.State.Paused}}')" = true ]; then
      docker unpause "$container_id" >/dev/null
    fi
  done
}

restore_problem_time_limit() {
  [ "$PROBLEM_LIMIT_CHANGED" -eq 1 ] || return 0
  "${RESTORE_COMPOSE[@]}" exec -T postgres psql -U codingjudge -d codingjudge -v ON_ERROR_STOP=1 \
    -c "UPDATE problems SET time_limit_ms = $ORIGINAL_PROBLEM_TIME_LIMIT WHERE id = 'echo';" >/dev/null
  PROBLEM_LIMIT_CHANGED=0
}

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  stop_capture
  info "restoring the dual-runtime development topology with two workers"
  unpause_project_containers || true
  if ! "${RESTORE_COMPOSE[@]}" up -d --scale worker=2 --wait --remove-orphans >/dev/null 2>&1; then
    warn "failed to restore the dual-runtime development topology"
    exit_code=1
  elif ! restore_problem_time_limit; then
    warn "failed to restore echo problem time limit to $ORIGINAL_PROBLEM_TIME_LIMIT ms"
    exit_code=1
  fi
  if [ "$exit_code" -ne 0 ]; then
    warn "executor topology benchmark failed with exit code $exit_code"
  fi
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

positive_integer() {
  local name=$1 value=$2
  case "$value" in ''|*[!0-9]*) die "$name must be a positive integer" ;; esac
  [ "$value" -gt 0 ] || die "$name must be greater than zero"
}

for pair in \
  "ET_BATCH_SIZE:$ET_BATCH_SIZE" \
  "ET_VUS:$ET_VUS" \
  "ET_DRAIN_TIMEOUT_SECONDS:$ET_DRAIN_TIMEOUT_SECONDS" \
  "ET_REPETITIONS:$ET_REPETITIONS" \
  "ET_WORKERS:$ET_WORKERS" \
  "ET_PROBLEM_TIME_LIMIT_MS:$ET_PROBLEM_TIME_LIMIT_MS"; do
  positive_integer "${pair%%:*}" "${pair#*:}"
done

command -v docker >/dev/null || die "docker is required"
command -v jq >/dev/null || die "jq is required"
command -v awk >/dev/null || die "awk is required"

set_benchmark_problem_time_limit() {
  [ "$PROBLEM_LIMIT_CHANGED" -eq 0 ] || return 0
  ORIGINAL_PROBLEM_TIME_LIMIT=$("${COMPOSE[@]}" exec -T postgres psql -U codingjudge -d codingjudge -tA -v ON_ERROR_STOP=1 \
    -c "SELECT time_limit_ms FROM problems WHERE id = 'echo';")
  positive_integer "echo problem time limit" "$ORIGINAL_PROBLEM_TIME_LIMIT"
  if [ "$ET_PROBLEM_TIME_LIMIT_MS" -lt "$ORIGINAL_PROBLEM_TIME_LIMIT" ]; then
    die "ET_PROBLEM_TIME_LIMIT_MS must not reduce the existing echo problem limit"
  fi
  "${COMPOSE[@]}" exec -T postgres psql -U codingjudge -d codingjudge -v ON_ERROR_STOP=1 \
    -c "UPDATE problems SET time_limit_ms = $ET_PROBLEM_TIME_LIMIT_MS WHERE id = 'echo';" >/dev/null
  PROBLEM_LIMIT_CHANGED=1
  info "temporarily raised echo problem limit from $ORIGINAL_PROBLEM_TIME_LIMIT ms to $ET_PROBLEM_TIME_LIMIT_MS ms"
}

set_topology() {
  local topology=$1
  case "$topology" in
    single)
      COMPOSE=("${BASE_COMPOSE[@]}" -f "$ROOT/deploy/compose/executor-node.yml" -f "$ROOT/deploy/compose/executor-benchmark.yml")
      EXECUTOR_SERVICES=(executor)
      EXPECTED_EXECUTORS=1
      EXPECTED_DAEMONS=1
      EXPECTED_SLOTS=1
      ;;
    shared)
      COMPOSE=("${BASE_COMPOSE[@]}" -f "$ROOT/deploy/compose/executor-node.yml" -f "$ROOT/deploy/compose/executor-shared-daemon.yml" -f "$ROOT/deploy/compose/executor-benchmark.yml")
      EXECUTOR_SERVICES=(executor executor-b)
      EXPECTED_EXECUTORS=2
      EXPECTED_DAEMONS=1
      EXPECTED_SLOTS=2
      ;;
    independent)
      COMPOSE=("${BASE_COMPOSE[@]}" -f "$ROOT/deploy/compose/multinode.yml" -f "$ROOT/deploy/compose/executor-benchmark.yml")
      EXECUTOR_SERVICES=(executor executor-b)
      EXPECTED_EXECUTORS=2
      EXPECTED_DAEMONS=2
      EXPECTED_SLOTS=2
      ;;
    *) die "unsupported topology $topology" ;;
  esac
}

queue_stats() {
  "${COMPOSE[@]}" exec -T redis redis-cli --raw XINFO GROUPS judge:submissions 2>/dev/null | awk '
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
  local run_id=$1
  "${COMPOSE[@]}" exec -T postgres psql -U codingjudge -d codingjudge -tA -F ',' -v ON_ERROR_STOP=1 -c "
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
  local summary=$1 label=$2
  [ -s "$summary" ] || die "$label: k6 summary is missing or empty"
  for metric in iterations submissions_created http_req_failed logical_failures http_req_duration; do
    jq -e ".metrics[\"$metric\"]" "$summary" >/dev/null ||
      die "$label: required metric '$metric' is missing"
  done
  local iterations created
  iterations=$(jq -r '.metrics.iterations.count // 0' "$summary")
  created=$(jq -r '.metrics.submissions_created.count // 0' "$summary")
  [ "$iterations" = "$ET_BATCH_SIZE" ] || die "$label: iterations=$iterations, want $ET_BATCH_SIZE"
  [ "$created" = "$ET_BATCH_SIZE" ] || die "$label: submissions=$created, want $ET_BATCH_SIZE"
  jq -e '.metrics.http_req_failed.value == 0' "$summary" >/dev/null || die "$label: HTTP failures are non-zero"
  jq -e '.metrics.logical_failures.value == 0' "$summary" >/dev/null || die "$label: logical failures are non-zero"
}

prometheus_scalar() {
  local encoded_query=$1
  "${COMPOSE[@]}" exec -T prometheus wget -qO- \
    "http://localhost:9090/api/v1/query?query=$encoded_query" 2>/dev/null |
    jq -r '.data.result[0].value[1] // 0' 2>/dev/null || printf '0\n'
}

slot_utilization() {
  prometheus_scalar 'sum(codingjudge_executor_executions_in_flight)%2Fsum(codingjudge_executor_slots)'
}

wait_for_prometheus_targets() {
  local attempt count
  for attempt in $(seq 1 20); do
    count=$(prometheus_scalar 'count(up%7Bjob%3D%22codingjudge-executor%22%7D%3D%3D1)')
    if [ "$count" = "$EXPECTED_EXECUTORS" ]; then
      return
    fi
    sleep 1
  done
  die "Prometheus sees ${count:-0} healthy executors, want $EXPECTED_EXECUTORS"
}

snapshot_executor_metrics() {
  local prefix=$1 service
  for service in "${EXECUTOR_SERVICES[@]}"; do
    "${COMPOSE[@]}" exec -T "$service" wget -qO- http://localhost:8090/metrics > "${prefix}-${service}.prom"
  done
}

combine_executor_metrics() {
  local prefix=$1 output=$2 service
  : > "$output"
  for service in "${EXECUTOR_SERVICES[@]}"; do
    cat "${prefix}-${service}.prom" >> "$output"
  done
}

metric_total() {
  local file=$1 metric=$2
  awk -v metric="$metric" '$1 ~ ("^" metric "({|$)") {sum += $NF} END {printf "%.0f\n", sum + 0}' "$file"
}

queue_p95_ms() {
  local before=$1 after=$2
  awk '
    function bucket(name, start, rest, finish) {
      start = index(name, "le=\"")
      if (start == 0) return ""
      rest = substr(name, start + 4)
      finish = index(rest, "\"")
      return substr(rest, 1, finish - 1)
    }
    FNR == NR && $1 ~ /^codingjudge_executor_queue_duration_seconds_bucket/ {
      before[bucket($1)] += $NF
      next
    }
    FNR != NR && $1 ~ /^codingjudge_executor_queue_duration_seconds_bucket/ {
      after[bucket($1)] += $NF
    }
    END {
      total = after["+Inf"] - before["+Inf"]
      if (total <= 0) exit 2
      threshold = total * 0.95
      count = split("0.001 0.005 0.01 0.025 0.05 0.1 0.25 0.5 1 2.5 5 10", bounds, " ")
      for (i = 1; i <= count; i++) {
        le = bounds[i]
        if (after[le] - before[le] >= threshold) {
          printf "%.3f\n", le * 1000
          exit
        }
      }
      exit 3
    }
  ' "$before" "$after" || die "executor queue P95 exceeded the finite histogram buckets or had no observations"
}

validate_live_topology() {
  local topology=$1 service id unique_daemons slots mounts
  local ids=()
  slots=0
  for service in "${EXECUTOR_SERVICES[@]}"; do
    id=$("${COMPOSE[@]}" exec -T "$service" docker info --format '{{.ID}}')
    [ -n "$id" ] || die "$topology: $service returned an empty daemon ID"
    ids+=("$id")
    mounts=$(docker inspect "$("${COMPOSE[@]}" ps -q "$service")" --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}')
    if grep -q '^/var/run/docker.sock$' <<< "$mounts"; then
      die "$topology: $service mounts the host Docker Socket"
    fi
    metrics=$("${COMPOSE[@]}" exec -T "$service" wget -qO- http://localhost:8090/metrics)
    slots=$((slots + $(awk '$1 == "codingjudge_executor_slots" {print int($2)}' <<< "$metrics")))
  done
  unique_daemons=$(printf '%s\n' "${ids[@]}" | sort -u | wc -l | tr -d ' ')
  [ "$unique_daemons" -eq "$EXPECTED_DAEMONS" ] ||
    die "$topology: found $unique_daemons unique daemon IDs, want $EXPECTED_DAEMONS"
  [ "$slots" -eq "$EXPECTED_SLOTS" ] || die "$topology: found $slots executor slots, want $EXPECTED_SLOTS"

  while IFS= read -r worker_id; do
    mounts=$(docker inspect "$worker_id" --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}')
    if grep -q '^/var/run/docker.sock$' <<< "$mounts"; then
      die "$topology: worker $worker_id mounts the Docker Socket"
    fi
  done < <("${COMPOSE[@]}" ps -q worker)

  printf 'topology=%s\nexecutors=%d\nunique_daemons=%d\ntotal_slots=%d\ndaemon_ids=%s\n' \
    "$topology" "$EXPECTED_EXECUTORS" "$unique_daemons" "$slots" "${ids[*]}"
}

start_capture() {
  local output=$1
  printf 'timestamp,pending,lag,outstanding,slot_utilization\n' > "$output"
  (
    while true; do
      local pending lag utilization
      read -r pending lag < <(queue_stats)
      utilization=$(slot_utilization)
      printf '%s,%s,%s,%s,%s\n' "$(date +%s)" "$pending" "$lag" "$((pending + lag))" "$utilization" >> "$output"
      sleep 1
    done
  ) &
  CAPTURE_PID=$!
}

run_round() {
  local trial=$1 topology=$2 code label run_id summary log samples evidence
  local before_prefix after_prefix before_all after_all
  local pre_pending pre_lag pending lag total accepted terminal makespan k6_exit drained
  local peak_pending peak_lag peak_outstanding mean_util peak_util throughput http_p95 queue_p95
  local before_a after_a before_b after_b delta_a delta_b

  case "$topology" in
    single) code=one ;;
    shared) code=shr ;;
    independent) code=ind ;;
  esac
  label="trial-$trial-$topology"
  set_topology "$topology"
  info "[$label] starting $EXPECTED_EXECUTORS executor(s), $EXPECTED_DAEMONS daemon(s), $EXPECTED_SLOTS slots"
  unpause_project_containers
  SUBMISSION_RATE_LIMIT_PER_MINUTE="$BENCH_RATE_LIMIT" \
  SUBMISSION_RATE_LIMIT_BURST="$BENCH_RATE_BURST" \
  EXECUTOR_MAX_CONCURRENCY=1 \
    "${COMPOSE[@]}" up -d --scale worker="$ET_WORKERS" --wait --remove-orphans api worker prometheus
  set_benchmark_problem_time_limit
  wait_for_prometheus_targets

  evidence="$RESULTS/executor-topology-t${trial}-${topology}-topology.txt"
  validate_live_topology "$topology" | tee "$evidence"
  read -r pre_pending pre_lag < <(queue_stats)
  if [ "$pre_pending" -ne 0 ] || [ "$pre_lag" -ne 0 ]; then
    die "$label: queue is not empty before round (pending=$pre_pending lag=$pre_lag)"
  fi

  before_prefix="$RESULTS/executor-topology-t${trial}-${topology}-before"
  after_prefix="$RESULTS/executor-topology-t${trial}-${topology}-after"
  before_all="${before_prefix}-all.prom"
  after_all="${after_prefix}-all.prom"
  snapshot_executor_metrics "$before_prefix"
  combine_executor_metrics "$before_prefix" "$before_all"

  run_id="et${trial}${code}$(date +%s)"
  summary="$RESULTS/executor-topology-t${trial}-${topology}.json"
  log="$RESULTS/executor-topology-t${trial}-${topology}.log"
  samples="$RESULTS/executor-topology-t${trial}-${topology}-samples.csv"
  rm -f "$summary" "$log"
  start_capture "$samples"

  info "[$label] submitting $ET_BATCH_SIZE Python jobs with $ET_VUS VUs"
  set +e
  SUBMISSION_RATE_LIMIT_PER_MINUTE="$BENCH_RATE_LIMIT" \
  SUBMISSION_RATE_LIMIT_BURST="$BENCH_RATE_BURST" \
  EXECUTOR_MAX_CONCURRENCY=1 \
    "${COMPOSE[@]}" --profile loadtest run --rm k6 \
    k6 run /scripts/saturation.js \
    --env "CJ_RUN_ID=$run_id" \
    --env "CJ_ITERATIONS=$ET_BATCH_SIZE" \
    --env "CJ_VUS=$ET_VUS" \
    --env "CJ_MAX_DURATION=$ET_MAX_DURATION" \
    --summary-export="/results/executor-topology-t${trial}-${topology}.json" 2>&1 | tee "$log"
  k6_exit=${PIPESTATUS[0]}
  set -e
  [ "$k6_exit" -eq 0 ] || die "$label: k6 exited with $k6_exit"
  validate_summary "$summary" "$label"

  total=0
  accepted=0
  terminal=0
  makespan=0
  for _ in $(seq 1 "$ET_DRAIN_TIMEOUT_SECONDS"); do
    IFS=',' read -r total accepted terminal makespan < <(query_run "$run_id")
    if [ "$total" -eq "$ET_BATCH_SIZE" ] && [ "$terminal" -eq "$ET_BATCH_SIZE" ]; then
      break
    fi
    sleep 1
  done
  if [ "$total" -ne "$ET_BATCH_SIZE" ] || [ "$terminal" -ne "$ET_BATCH_SIZE" ]; then
    die "$label: drain timed out (total=$total terminal=$terminal expected=$ET_BATCH_SIZE)"
  fi
  [ "$accepted" -eq "$ET_BATCH_SIZE" ] || die "$label: accepted=$accepted, want $ET_BATCH_SIZE"

  drained=0
  for _ in $(seq 1 20); do
    read -r pending lag < <(queue_stats)
    if [ "$pending" -eq 0 ] && [ "$lag" -eq 0 ]; then
      drained=1
      break
    fi
    sleep 1
  done
  [ "$drained" -eq 1 ] || die "$label: queue did not drain (pending=$pending lag=$lag)"
  stop_capture

  snapshot_executor_metrics "$after_prefix"
  combine_executor_metrics "$after_prefix" "$after_all"
  before_a=$(metric_total "${before_prefix}-executor.prom" codingjudge_executor_executions_total)
  after_a=$(metric_total "${after_prefix}-executor.prom" codingjudge_executor_executions_total)
  delta_a=$((after_a - before_a))
  delta_b=0
  if [ "$EXPECTED_EXECUTORS" -eq 2 ]; then
    before_b=$(metric_total "${before_prefix}-executor-b.prom" codingjudge_executor_executions_total)
    after_b=$(metric_total "${after_prefix}-executor-b.prom" codingjudge_executor_executions_total)
    delta_b=$((after_b - before_b))
  fi
  [ $((delta_a + delta_b)) -eq "$ET_BATCH_SIZE" ] ||
    die "$label: executor batches=$((delta_a + delta_b)), want $ET_BATCH_SIZE"
  [ "$delta_a" -gt 0 ] || die "$label: executor A received no batches"
  if [ "$EXPECTED_EXECUTORS" -eq 2 ] && [ "$delta_b" -le 0 ]; then
    die "$label: executor B received no batches"
  fi

  peak_pending=$(awk -F, 'NR > 1 && $2 + 0 > max {max=$2 + 0} END {print max + 0}' "$samples")
  peak_lag=$(awk -F, 'NR > 1 && $3 + 0 > max {max=$3 + 0} END {print max + 0}' "$samples")
  peak_outstanding=$(awk -F, 'NR > 1 && $4 + 0 > max {max=$4 + 0} END {print max + 0}' "$samples")
  mean_util=$(awk -F, 'NR > 1 && ($4 + 0 > 0 || $5 + 0 > 0) {sum += $5 + 0; count++} END {if (count == 0) exit 1; printf "%.6f", sum/count}' "$samples")
  peak_util=$(awk -F, 'NR > 1 && $5 + 0 > max {max=$5 + 0} END {printf "%.6f", max + 0}' "$samples")
  [ "$peak_lag" -gt 0 ] || die "$label: no positive Stream lag was observed; increase ET_BATCH_SIZE"
  awk -v value="$peak_util" 'BEGIN {exit !(value > 0)}' || die "$label: no positive executor slot utilization was observed"
  awk -v value="$makespan" 'BEGIN {exit !(value > 0)}' || die "$label: invalid makespan $makespan"
  throughput=$(awk -v batch="$ET_BATCH_SIZE" -v seconds="$makespan" 'BEGIN {printf "%.6f", batch/seconds}')
  http_p95=$(jq -r '.metrics.http_req_duration["p(95)"] // 0' "$summary")
  queue_p95=$(queue_p95_ms "$before_all" "$after_all")

  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$trial" "$topology" "$EXPECTED_EXECUTORS" "$EXPECTED_DAEMONS" "$EXPECTED_SLOTS" \
    "$total" "$accepted" "$makespan" "$throughput" "$http_p95" "$peak_pending" "$peak_lag" \
    "$peak_outstanding" "$queue_p95" "$mean_util" "$peak_util" "$delta_a" "$delta_b" >> "$CSV"
  info "[$label] complete: ${throughput}/s, makespan=${makespan}s, executor batches=$delta_a/$delta_b"
}

mkdir -p "$RESULTS" "$(dirname "$REPORT")"
info "building application images once before the controlled rounds"
"${RESTORE_COMPOSE[@]}" build api worker executor executor-b migrate judge-images judge-images-b
info "stopping UI-only services during the benchmark"
unpause_project_containers
"${BASE_COMPOSE[@]}" stop frontend grafana >/dev/null 2>&1 || true

DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)
MEMORY=$(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%.0f GB", $1/1024/1024/1024}' || echo unknown)
TRACKED_TREE=dirty
if git -C "$ROOT" diff --quiet && git -C "$ROOT" diff --cached --quiet; then
  TRACKED_TREE=clean
fi
{
  printf 'date: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'git_commit: %s\n' "$(git -C "$ROOT" rev-parse --short HEAD)"
  printf 'git_tracked_tree: %s\n' "$TRACKED_TREE"
  printf 'os: %s\n' "$(uname -s)"
  printf 'arch: %s\n' "$(uname -m)"
  printf 'logical_cpus: %s\n' "$(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo unknown)"
  printf 'memory: %s\n' "$MEMORY"
  printf 'docker_version: %s\n' "$DOCKER_VERSION"
  printf 'dind_image: docker:27.5.1-dind\n'
  printf 'batch_size: %s\n' "$ET_BATCH_SIZE"
  printf 'vus: %s\n' "$ET_VUS"
  printf 'language: python\nproblem_id: echo\n'
  printf 'workers: %s\nworker_concurrency: 1\n' "$ET_WORKERS"
  printf 'problem_time_limit_ms: %s\n' "$ET_PROBLEM_TIME_LIMIT_MS"
  printf 'repetitions: %s\n' "$ET_REPETITIONS"
} > "$META"

printf 'trial,topology,executors,daemons,total_slots,batch,accepted,makespan_seconds,throughput_per_second,http_p95_ms,peak_pending,peak_lag,peak_outstanding,executor_queue_p95_ms,mean_slot_utilization,peak_slot_utilization,executor_a_batches,executor_b_batches\n' > "$CSV"

for trial in $(seq 1 "$ET_REPETITIONS"); do
  case $(((trial - 1) % 3)) in
    0) order=(single shared independent) ;;
    1) order=(independent single shared) ;;
    2) order=(shared independent single) ;;
  esac
  info "trial $trial/$ET_REPETITIONS order: ${order[*]}"
  for topology in "${order[@]}"; do
    run_round "$trial" "$topology"
  done
done

info "rendering the validated technical report"
rm -f "$REPORT_TMP"
(cd "$ROOT" && go run ./scripts/executor-topology-report "$META" "$CSV") > "$REPORT_TMP"
[ -s "$REPORT_TMP" ] || die "generated report is empty"
mv "$REPORT_TMP" "$REPORT"
info "executor topology benchmark complete: $REPORT"
