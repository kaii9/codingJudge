# Executor Topology Benchmark Implementation Plan

## Goal

Measure whether independent Docker daemons improve judge backlog-drain capacity, while separating that effect from adding executor processes and execution slots.

## Controlled topologies

All topologies use `docker:27.5.1-dind`, four workers with concurrency one, the same Python `echo` workload, the same runtime images, and the same host. During the benchmark, the runner temporarily raises the `echo` problem limit to 10 seconds and restores it on exit. This keeps container-startup latency in the measured makespan without misclassifying correct benchmark code when a saturated daemon takes more than the normal 2.5-second outer window to launch a sandbox.

| ID | Executors | Docker daemons | Total slots | Question |
| --- | ---: | ---: | ---: | --- |
| `single` | 1 | 1 | 1 | Baseline capacity |
| `shared` | 2 | 1 | 2 | Does another executor process help while the daemon remains shared? |
| `independent` | 2 | 2 | 2 | What changes when the same executor capacity uses independent daemons? |

Each executor is deliberately limited to one slot. A preliminary two-executor/four-slot shared-daemon run produced three sandbox startup timeouts out of 18 submissions because Docker launch latency crossed the problem's 2.5-second outer deadline. The controlled comparison therefore uses the highest tested shared-daemon concurrency that preserves the all-accepted validity gate.

## Tasks

1. Add a tested Go report generator that validates complete balanced trials, topology invariants, accepted counts, observed queue pressure, executor distribution, and bounded utilization metrics.
2. Add Compose overlays for one DinD runtime and for two executors sharing that runtime; reuse the existing multi-node overlay for independent daemons.
3. Add a benchmark runner that cycles topologies in balanced order, validates daemon identities, submits a fixed burst, captures queue and executor metrics, saves raw evidence, and refuses to publish incomplete results.
4. Add `make load-executor-topologies`, Compose/config checks, and documentation.
5. Run a short validation, then the default three-by-three benchmark, generate the dated Markdown report, and review claims against the raw CSV.
6. Run full tests, race/static/config checks, inspect the diff, and commit only phase-one files.

## Report contract

The technical report must lead with the measured result, define makespan and throughput, show aggregate and individual runs, distinguish `shared` versus `independent` as the controlled comparison, and state that all daemons still share one Docker Desktop VM and host resources. A compact table is preferred over a chart because there are only three categorical topologies and exact values matter more than a visual trend.

## Stop conditions

- Any round has HTTP or logical failures.
- Accepted count differs from the requested batch.
- Redis Pending or Lag is non-zero before or after a round.
- No positive queue pressure is observed.
- No positive executor slot utilization is observed.
- Topology daemon identities or executor request distribution contradict the declared topology.
- The benchmark cannot finish within the configured drain timeout.
