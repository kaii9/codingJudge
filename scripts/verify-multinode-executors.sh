#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
COMPOSE=(docker compose -f "$ROOT/docker-compose.yml" -f "$ROOT/deploy/compose/multinode.yml")

COOKIE=""

info() { printf '[info] %s\n' "$*"; }
die() { printf '[fatal] %s\n' "$*" >&2; exit 1; }

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  if [ -n "$COOKIE" ] && [ -f "$COOKIE" ]; then
    rm "$COOKIE"
  fi
  "${COMPOSE[@]}" up -d --no-deps --scale worker=2 worker >/dev/null 2>&1 || true
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || die "docker is required"
command -v curl >/dev/null || die "curl is required"
command -v jq >/dev/null || die "jq is required"

execution_count() {
  "${COMPOSE[@]}" exec -T "$1" wget -qO- http://localhost:8090/metrics |
    awk '/^codingjudge_executor_executions_total/ {sum += $NF} END {print sum + 0}'
}

queue_stats() {
  local pending lag
  pending=$("${COMPOSE[@]}" exec -T redis redis-cli XPENDING judge:submissions judge-workers | sed -n '1p')
  lag=$("${COMPOSE[@]}" exec -T redis redis-cli XINFO GROUPS judge:submissions |
    awk '$1 == "lag" {getline; print; exit}')
  printf '%s %s\n' "${pending:-0}" "${lag:-0}"
}

submit_and_wait() {
  local sequence=$1 language=$2 code=$3 body response id judge_status attempt
  body=$(jq -nc --arg language "$language" --arg code "$code" \
    '{problemId:"sum",language:$language,code:$code}')
  response=$(curl -fsS -b "$COOKIE" -X POST http://localhost:18080/submissions \
    -H 'Content-Type: application/json' \
    -H "Idempotency-Key: multinode-$USERNAME-$sequence" \
    -d "$body")
  id=$(jq -r '.id' <<<"$response")
  judge_status=queued
  for attempt in $(seq 1 60); do
    response=$(curl -fsS -b "$COOKIE" "http://localhost:18080/submissions/$id")
    judge_status=$(jq -r '.status' <<<"$response")
    case "$judge_status" in
      accepted|wrong_answer|compile_error|runtime_error|time_limit_exceeded|internal_error) break ;;
    esac
    sleep 1
  done
  [ "$judge_status" = accepted ] || die "submission $id ended as $judge_status"
  info "$language submission accepted: $id"
}

info "checking independent Docker daemon identities"
DAEMON_A=$("${COMPOSE[@]}" exec -T executor docker info --format '{{.ID}}')
DAEMON_B=$("${COMPOSE[@]}" exec -T executor-b docker info --format '{{.ID}}')
[ -n "$DAEMON_A" ] && [ -n "$DAEMON_B" ] || die "Docker daemon ID is empty"
[ "$DAEMON_A" != "$DAEMON_B" ] || die "executors point to the same Docker daemon"
info "daemon A: $DAEMON_A"
info "daemon B: $DAEMON_B"

info "checking that workers do not mount the Docker Socket"
while IFS= read -r worker_id; do
  if docker inspect "$worker_id" --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' |
    grep -q '/var/run/docker.sock'; then
    die "worker $worker_id mounts the Docker Socket"
  fi
done < <("${COMPOSE[@]}" ps -q worker)

info "checking that executors do not mount the host Docker Socket"
for service in executor executor-b; do
  executor_id=$("${COMPOSE[@]}" ps -q "$service")
  [ -n "$executor_id" ] || die "$service is not running"
  if docker inspect "$executor_id" --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' |
    grep -q '/var/run/docker.sock'; then
    die "$service mounts the host Docker Socket"
  fi
done

info "using one worker to make round-robin distribution deterministic"
"${COMPOSE[@]}" up -d --no-deps --scale worker=1 worker >/dev/null
BEFORE_A=$(execution_count executor)
BEFORE_B=$(execution_count executor-b)

COOKIE=$(mktemp /tmp/codingjudge-multinode-verify.XXXXXX)
USERNAME="multinode$(date +%s)"
REGISTER_BODY=$(jq -nc --arg username "$USERNAME" --arg password 'multinode-verify-password' \
  '{username:$username,password:$password}')
curl -fsS -c "$COOKIE" -X POST http://localhost:18080/auth/register \
  -H 'Content-Type: application/json' -d "$REGISTER_BODY" >/dev/null

submit_and_wait 1 python 'a, b = map(int, input().split()); print(a + b)'
submit_and_wait 2 go 'package main

import "fmt"

func main() {
	var a, b int
	fmt.Scan(&a, &b)
	fmt.Println(a + b)
}'
submit_and_wait 3 cpp '#include <iostream>

int main() {
    long long a, b;
    std::cin >> a >> b;
    std::cout << a + b << std::endl;
    return 0;
}'

AFTER_A=$(execution_count executor)
AFTER_B=$(execution_count executor-b)
DELTA_A=$((AFTER_A - BEFORE_A))
DELTA_B=$((AFTER_B - BEFORE_B))
[ "$DELTA_A" -ge 1 ] || die "executor A did not receive a batch"
[ "$DELTA_B" -ge 1 ] || die "executor B did not receive a batch"
[ $((DELTA_A + DELTA_B)) -eq 3 ] ||
  die "executor execution delta is $((DELTA_A + DELTA_B)), want 3"
info "execution deltas: executor A=$DELTA_A, executor B=$DELTA_B"

read -r PENDING LAG < <(queue_stats)
[ "$PENDING" -eq 0 ] && [ "$LAG" -eq 0 ] ||
  die "queue did not drain (pending=$PENDING lag=$LAG)"

for attempt in $(seq 1 10); do
  PROMETHEUS_TARGETS=$(curl -fsSG http://localhost:9090/api/v1/query \
    --data-urlencode 'query=count(up{job="codingjudge-executor"} == 1)' |
    jq -r '.data.result[0].value[1] // 0')
  [ "$PROMETHEUS_TARGETS" -eq 2 ] && break
  sleep 1
done
[ "$PROMETHEUS_TARGETS" -eq 2 ] ||
  die "Prometheus sees $PROMETHEUS_TARGETS healthy executor targets, want 2"

info "multi-node executor verification passed"
