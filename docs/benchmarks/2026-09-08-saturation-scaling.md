# Worker Saturation and Backlog-Drain Benchmark

- **Date:** 2026-09-08T14:22:00Z
- **Git commit:** `ec12d9d` (clean tracked tree)
- **Host:** Darwin/arm64, 8 logical CPUs, 16 GB memory
- **Docker:** 29.1.3
- **Workload:** 60 `echo` python submissions, 20 VUs, shared-iterations burst; worker concurrency 1
- **Samples per worker count:** 3

The burst intentionally creates Redis Stream lag before the workers drain it. Makespan is measured from the earliest submission creation to the latest terminal update for that isolated run ID.

| Workers | Runs | Accepted | Median makespan | Median throughput | Throughput range | vs 1 worker | Scaling efficiency | Median HTTP P95 | Max backlog |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 3 | 180/180 | 20.417s | 2.939/s | 2.434–3.056/s | 1.00x | 100.0% | 407.48ms | 59 |
| 2 | 3 | 180/180 | 19.577s | 3.065/s | 2.718–3.207/s | 1.04x | 52.1% | 505.48ms | 59 |
| 4 | 3 | 180/180 | 17.942s | 3.344/s | 2.982–3.572/s | 1.14x | 28.4% | 416.87ms | 60 |

### Individual runs

| Trial | Workers | Accepted | Makespan | Throughput | HTTP P95 | Peak Pending | Peak Lag | Peak backlog |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 60/60 | 24.646s | 2.434/s | 546.63ms | 1 | 58 | 59 |
| 1 | 2 | 60/60 | 18.711s | 3.207/s | 505.48ms | 2 | 57 | 59 |
| 1 | 4 | 60/60 | 16.800s | 3.572/s | 409.73ms | 4 | 55 | 59 |
| 2 | 1 | 60/60 | 19.632s | 3.056/s | 400.05ms | 1 | 58 | 59 |
| 2 | 2 | 60/60 | 19.577s | 3.065/s | 447.79ms | 2 | 56 | 58 |
| 2 | 4 | 60/60 | 17.942s | 3.344/s | 518.91ms | 4 | 56 | 60 |
| 3 | 1 | 60/60 | 20.417s | 2.939/s | 407.48ms | 1 | 57 | 58 |
| 3 | 2 | 60/60 | 22.073s | 2.718/s | 588.48ms | 2 | 56 | 58 |
| 3 | 4 | 60/60 | 20.117s | 2.982/s | 416.87ms | 4 | 56 | 60 |

## Interpretation

Every run processed the same batch and reached terminal `accepted` with zero HTTP and logical failures. Peak Stream lag above zero demonstrates that the workers were presented with queued work rather than an underloaded steady state. Comparisons use median throughput. The 4-worker configuration delivered only 1.14x the one-worker throughput (28.4% scaling efficiency). The gain is real but far below proportional scaling, indicating a shared bottleneck.

This measures backlog-drain capacity on the recorded machine, not a universal production QPS limit. Docker Desktop, host scheduling, image caching, PostgreSQL, Redis, and the selected Python workload all affect the result. Repeated trials reduce one-off noise but are still insufficient for production capacity planning.

## Reproduce

```bash
make load-saturation
```

The runner refuses to publish a report unless every round has the exact requested submission count, zero HTTP and logical failures, all submissions accepted, observed positive Stream lag, and a fully drained queue.
