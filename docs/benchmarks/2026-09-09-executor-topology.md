# Executor Topology Saturation Benchmark

## Technical summary

The independent-daemon topology delivered 0.94x versus the shared-daemon topology and 1.40x versus the single-executor baseline on this recorded host. Adding a second executor while retaining one daemon produced 1.50x the baseline throughput. These are descriptive local measurements, not production capacity claims.

## The controlled comparison isolates Docker daemon topology

All three configurations used the same DinD image, workload, worker count, runtime images, and host. `shared` and `independent` expose the same executor slot count, so their comparison changes the Docker daemon count without changing declared executor capacity.

| Topology | Executors | Daemons | Slots | Runs | Accepted | Median makespan | Median throughput | Throughput range | vs single | Median HTTP P95 | Executor queue P95 | Mean slot utilization | Max backlog |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| single | 1 | 1 | 1 | 3 | 180/180 | 14.062s | 4.267/s | 4.247–4.300/s | 1.00x | 884.31ms | ≤ 2500.00ms | 100.0% | 58 |
| shared | 2 | 1 | 2 | 3 | 180/180 | 9.390s | 6.390/s | 6.179–6.645/s | 1.50x | 679.15ms | ≤ 1000.00ms | 92.9% | 57 |
| independent | 2 | 2 | 2 | 3 | 180/180 | 10.036s | 5.979/s | 5.306–6.353/s | 1.40x | 607.06ms | ≤ 1000.00ms | 92.9% | 56 |

The small categorical comparison is shown as an exact table rather than a chart: three topologies do not provide a meaningful trend, while exact throughput and topology invariants are central to auditing the result.

## Individual runs show the observed variation

| Trial | Topology | Accepted | Makespan | Throughput | HTTP P95 | Peak Pending | Peak Lag | Peak backlog | Executor queue P95 | Mean/peak slot utilization | Executor A/B batches |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | single | 60/60 | 14.062s | 4.267/s | 646.63ms | 4 | 54 | 58 | ≤ 2500.00ms | 100.0% / 100.0% | 60 / 0 |
| 1 | shared | 60/60 | 9.030s | 6.645/s | 679.15ms | 4 | 53 | 57 | ≤ 1000.00ms | 100.0% / 100.0% | 31 / 29 |
| 1 | independent | 60/60 | 10.036s | 5.979/s | 558.67ms | 4 | 52 | 56 | ≤ 1000.00ms | 92.9% / 100.0% | 30 / 30 |
| 2 | single | 60/60 | 13.953s | 4.300/s | 884.31ms | 4 | 54 | 58 | ≤ 2500.00ms | 100.0% / 100.0% | 60 / 0 |
| 2 | shared | 60/60 | 9.710s | 6.179/s | 886.37ms | 4 | 53 | 57 | ≤ 2500.00ms | 92.9% / 100.0% | 32 / 28 |
| 2 | independent | 60/60 | 11.308s | 5.306/s | 607.06ms | 4 | 52 | 56 | ≤ 1000.00ms | 100.0% / 100.0% | 30 / 30 |
| 3 | single | 60/60 | 14.127s | 4.247/s | 990.44ms | 4 | 54 | 58 | ≤ 1000.00ms | 100.0% / 100.0% | 60 / 0 |
| 3 | shared | 60/60 | 9.390s | 6.390/s | 605.78ms | 4 | 53 | 57 | ≤ 1000.00ms | 85.7% / 100.0% | 28 / 32 |
| 3 | independent | 60/60 | 9.445s | 6.353/s | 761.23ms | 4 | 51 | 55 | ≤ 1000.00ms | 92.9% / 100.0% | 32 / 28 |

## Measurement definitions and method

- **Workload:** 60 Python submissions to `echo`, submitted by 20 k6 VUs; 4 workers with concurrency 1.
- **Correctness isolation:** the runner uses a temporary 10000 ms problem limit so container-startup latency remains visible in makespan without becoming a solution TLE, then restores the original limit.
- **Makespan:** elapsed database time from the earliest submission creation to the latest terminal update for one isolated run ID.
- **Throughput:** accepted batch size divided by makespan; comparisons use the median across balanced-order trials.
- **Executor queue P95:** the upper bound of the first Prometheus histogram bucket containing at least 95% of slot-wait observations during the round.
- **Slot utilization:** one-second Prometheus samples of total in-flight batches divided by configured slots; the mean uses samples where queue work or slot use was present.
- **Per-node concurrency:** one slot per executor. A preliminary four-slot shared-daemon run produced sandbox startup timeouts, so the controlled benchmark uses the highest tested shared-daemon setting that preserves accepted-result validity.
- **Validity gates:** exact requested submissions, zero HTTP/logical failures, all accepted, positive Stream lag and slot use, correct daemon identities, both executors used where declared, and zero final Pending/Lag.

Recorded at 2026-09-09T09:00:33Z from commit `70d0e9a` (clean tracked tree) on Darwin/arm64 with 8 logical CPUs and 16 GB memory. Docker 29.1.3; runtime node image `docker:27.5.1-dind`.

## Limitations and robustness

The two DinD daemons have separate Docker state and sandbox work volumes but still share the same Docker Desktop VM and host resources, including CPU and physical storage. The result therefore isolates daemon state better than the earlier shared-socket benchmark, but it does not measure two independent machines or prove a universal production QPS limit. Python avoids compile-stage variance; a mixed-language capacity test may rank the topologies differently. Balanced order and repeated medians reduce one-off cache and thermal effects but do not eliminate them.

## Recommended next step

Repeat the same workload on two independent VM hosts before choosing production capacity, then implement health-aware executor removal and recovery so additional capacity also improves availability.

## Reproduce

```bash
make load-executor-topologies
```

The runner retains per-round k6 summaries, queue/utilization samples, topology evidence, and the validated CSV under `loadtest/results/`. It refuses to replace this report when any validity gate fails.
