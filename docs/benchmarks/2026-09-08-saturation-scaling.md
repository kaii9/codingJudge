# Worker Saturation and Backlog-Drain Benchmark

- **Date:** 2026-09-08T13:05:13Z
- **Git commit:** `2a27e66` (clean tracked tree)
- **Host:** Darwin/arm64, 8 logical CPUs, 16 GB memory
- **Docker:** 29.1.3
- **Workload:** 60 `echo` python submissions, 20 VUs, shared-iterations burst; worker concurrency 1
- **Samples:** 1 run per worker count

The burst intentionally creates Redis Stream lag before the workers drain it. Makespan is measured from the earliest submission creation to the latest terminal update for that isolated run ID.

| Workers | Batch | Accepted | Drain makespan | Throughput | vs 1 worker | Scaling efficiency | HTTP P95 | Peak Pending | Peak Lag | Peak outstanding |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 60 | 60 | 17.752s | 3.380/s | 1.00x | 100.0% | 297.89ms | 1 | 57 | 58 |
| 2 | 60 | 60 | 28.146s | 2.132/s | 0.63x | 31.5% | 492.71ms | 2 | 56 | 58 |
| 4 | 60 | 60 | 20.736s | 2.894/s | 0.86x | 21.4% | 398.99ms | 4 | 56 | 60 |

## Interpretation

All rounds processed the same batch and reached terminal `accepted` with zero HTTP and logical failures. Peak Stream lag above zero demonstrates that the workers were presented with queued work rather than an underloaded steady state. The 4-worker run regressed to 0.86x the one-worker throughput (21.4% scaling efficiency), so this run does not demonstrate horizontal scaling. This is consistent with contention in a shared execution resource, such as concurrent sandbox launches through one Docker daemon, but this benchmark alone does not prove the cause.

This measures backlog-drain capacity on the recorded machine, not a universal production QPS limit. Docker Desktop, host scheduling, image caching, PostgreSQL, Redis, and the selected Python workload all affect the result. Each worker count was sampled once; repeat trials are required before using these numbers for capacity planning.

## Reproduce

```bash
make load-saturation
```

The runner refuses to publish a report unless every round has the exact requested submission count, zero HTTP and logical failures, all submissions accepted, observed positive Stream lag, and a fully drained queue.
