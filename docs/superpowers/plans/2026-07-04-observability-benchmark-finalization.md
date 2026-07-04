# Observability Benchmark Finalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the observability/load-testing phase with strict fixed-load validation, truthful benchmark reporting, synchronized documentation, and a clean feature branch.

**Architecture:** Keep the existing monitoring architecture. Tighten the k6 runner and Go renderer so invalid runs cannot generate a report, expose HTTP and business failures separately, rerun the 1/2/4-worker benchmark, and update documentation only from measured evidence.

**Tech Stack:** Go 1.25, Bash, k6 2.0.0, Docker Compose, Redis Streams, Prometheus, Grafana.

---

## Constraints

- Work in `/Users/kai/Downloads/go_project/codingJudge/.claude/worktrees/codex+observability-load-testing`.
- Stay on `worktree-codex+observability-load-testing`; do not merge or modify `main`.
- Read `AGENTS.md` and `docs/superpowers/plans/2026-07-03-observability-load-testing.md`.
- Preserve unrelated changes. Do not reset or rebase.
- A valid round requires accepted count equal to created count, zero logical failures, zero HTTP failures, and zero dropped iterations.
- This is fixed-load latency/queue testing, not a maximum-throughput or linear-scaling benchmark.

### Task 1: Add Strict Renderer Tests

**Files:**
- Modify: `scripts/render_benchmark_report_test.go`
- Modify: `scripts/testdata/*.json`

- [ ] Verify branch and existing dirty state:

```bash
git branch --show-current
git status --short
```

- [ ] Add a failing test where created and iterations are 120 but accepted is 119. Require non-zero renderer exit.
- [ ] Add a failing test where `logical_failures.value=0.0082644628` while `http_req_failed.value=0`. Keep other metrics valid and require non-zero exit.
- [ ] Add a failing test where offered rate is 1/s for 2m but created/iterations are 100. Require non-zero exit.
- [ ] Update successful output assertions to require:

```text
| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | HTTP failure | Logical failure | Peak pending (sampled) |
```

- [ ] Verify RED:

```bash
go test ./scripts -run 'TestRender' -v
```

Expected: the new strict assertions fail against the current renderer.

### Task 2: Enforce Strict Renderer Validation

**Files:**
- Modify: `scripts/render-benchmark-report.go`
- Test: `scripts/render_benchmark_report_test.go`

- [ ] Replace the 95% accepted threshold with:

```go
if math.Abs(acceptedCount-createdCount) > 0.5 {
    errs = append(errs, fmt.Sprintf("accepted=%.0f != created=%.0f", acceptedCount, createdCount))
}
```

- [ ] Read and validate separate failure rates:

```go
httpFailureRate := mustFloat("http_req_failed", "value")
logicalFailureRate := mustFloat("logical_failures", "value")
```

Reject either value when greater than zero. Include both in row data, NaN/Inf checks, and report columns.

- [ ] Tighten arrival-count validation:

```go
expected := offeredRate * durationSecs
tolerance := math.Max(2, expected*0.02)
if math.Abs(createdCount-expected) > tolerance {
    errs = append(errs, fmt.Sprintf(
        "created=%.0f outside expected range [%.0f, %.0f]",
        createdCount, expected-tolerance, expected+tolerance,
    ))
}
```

Reject zero, negative, NaN, or infinite rate/duration.

- [ ] Run and commit:

```bash
go test ./scripts -v
go test ./...
git add scripts/render-benchmark-report.go scripts/render_benchmark_report_test.go scripts/testdata
git commit -m "fix: enforce strict benchmark result validation"
```

Expected: all tests pass.

### Task 3: Make The Runner Self-Verifying

**Files:**
- Modify: `scripts/run-worker-scale-benchmark.sh`
- Modify: `.gitignore`
- Delete from Git: `docs/benchmarks/2026-07-03-worker-scaling.md.tmp`

- [ ] Capture each k6 run with `tee` and preserve the k6 status:

```bash
K6_LOG="$RESULTS/k6-worker-${workers}.log"
set +e
docker compose --profile loadtest run --rm k6 k6 run   "/scripts/$SCENARIO" $K6_ARGS   --summary-export="/results/k6-worker-${workers}.json" 2>&1 | tee "$K6_LOG"
k6_exit=${PIPESTATUS[0]}
set -e
```

Reject unless the log contains `constant-arrival-rate`. Reject if it contains `overrode scenarios configuration entirely`.

- [ ] Require in shell validation:

```text
created == iterations
accepted == created
logical_failures.value == 0
http_req_failed.value == 0
dropped_iterations.count == 0
abs(created - rate*duration) <= max(2, rate*duration*0.02)
```

- [ ] Replace the `sed | bc` duration conversion with explicit integer `s`, `m`, and `h` handling. Unsupported formats must call `die`.

- [ ] Add ignore rules:

```gitignore
loadtest/results/*.log
docs/benchmarks/*.tmp
```

- [ ] Remove the tracked temporary file and verify:

```bash
git rm --ignore-unmatch docs/benchmarks/2026-07-03-worker-scaling.md.tmp
bash -n scripts/run-worker-scale-benchmark.sh
docker compose --profile loadtest config --quiet
git diff --check
```

- [ ] Commit:

```bash
git add scripts/run-worker-scale-benchmark.sh .gitignore
git add -u docs/benchmarks
git commit -m "fix: reject invalid fixed-load benchmark runs"
```

### Task 4: Generate A New Valid Benchmark

**Files:**
- Regenerate: `docs/benchmarks/2026-07-03-worker-scaling.md`
- Generated and ignored: `loadtest/results/k6-worker-{1,2,4}.{json,log}`

- [ ] Start a known environment and confirm initial Pending is zero:

```bash
docker compose up -d --build --scale worker=2
docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers
```

- [ ] Run all rounds:

```bash
make load-worker-scale
```

Every round must show `constant-arrival-rate` and pass strict validation. If one round fails, diagnose it and rerun all rounds. Never hand-edit measured values.

- [ ] Inspect raw summaries:

```bash
jq '{iterations:.metrics.iterations.count,created:.metrics.submissions_created.count,accepted:.metrics.submissions_accepted.count,dropped:(.metrics.dropped_iterations.count // 0),http_failure:.metrics.http_req_failed.value,logical_failure:.metrics.logical_failures.value}' loadtest/results/k6-worker-{1,2,4}.json
```

Expected: created equals iterations and accepted; dropped and both failure values are zero.

- [ ] Confirm final Pending:

```bash
docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers
```

Expected: `0`.

### Task 5: Synchronize Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/development-plan.md`
- Verify: `docs/benchmarks/2026-07-03-worker-scaling.md`

- [ ] Remove obsolete claims:

```text
1.33 -> 3.34 accepted/s
5530 -> 2062ms
45 -> 53 req/s
2356 -> 1042ms
```

Verify:

```bash
rg -n '1\.33|3\.34|5530|2062|45.*53|2356|1042' README.md docs
```

Expected after editing: no obsolete claim remains.

- [ ] Change README stack status to:

```text
Observability: slog + Prometheus + Grafana
```

- [ ] Update Phase 8 only from the regenerated report. State that it compares latency, failures, and sampled Pending under identical fixed load and does not measure maximum throughput.
- [ ] Only claim zero failures when both HTTP and logical failures are zero in every raw summary.
- [ ] Verify and commit:

```bash
rg -n 'planned|Failure rate|HTTP failure|Logical failure|constant-arrival-rate|maximum throughput' README.md docs/development-plan.md docs/benchmarks
git diff --check
git add README.md docs/development-plan.md docs/benchmarks/2026-07-03-worker-scaling.md
git commit -m "docs: publish validated fixed-load benchmark"
```

### Task 6: Final Verification

- [ ] Backend checks:

```bash
make test
go test -race ./...
go vet ./...
```

- [ ] Integration checks:

```bash
TEST_DATABASE_URL='postgres://codingjudge:codingjudge@localhost:15432/codingjudge_test?sslmode=disable' TEST_REDIS_ADDR='localhost:16379' go test -tags=integration ./internal/store ./internal/queue
```

- [ ] Observability and smoke checks:

```bash
make observability-config
bash scripts/verify-grafana.sh
make load-smoke
```

- [ ] Final repository checks:

```bash
docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers
git diff --check
git status --short --branch
git log --oneline --decorate --max-count=8
```

Expected: Pending is zero, no generated artifacts are tracked, worktree is clean, all work is committed on the feature branch, and `main` remains untouched.

- [ ] Final response must report commit hashes, exact verification results, raw counts for 1/2/4 workers, HTTP P95, Judge P95, HTTP failure, logical failure, sampled Peak Pending, final Redis Pending, and the Python-only macOS limitation.

Do not claim linear scaling or maximum throughput.

