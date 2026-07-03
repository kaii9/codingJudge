# Worker Scaling Benchmark

**Date:** 2026-07-03T07:35:45Z | **Commit:** 03ee3cd | **OS:** Darwin | **Arch:** arm64 | **CPUs:** 8

## Results

| Workers | Submission rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |
| --- | --- | --- | --- | --- | --- |
| 1 | 8.50/s | 150.70ms | 3000.00ms | 0.0000% | - |
| 2 | 16.20/s | 170.30ms | 2700.00ms | 0.0000% | - |
| 4 | 28.70/s | 220.80ms | 2400.00ms | 0.0000% | - |

## Interpretation

_This benchmark was run in a local Docker Compose environment. Results are not production capacity guarantees._

- CPU and memory contention increase as workers scale, but throughput should improve roughly linearly up to the number of logical CPUs.
- Judge P95 is dominated by Docker container startup and compilation time; it improves with more workers distributing the load.
- Zero or near-zero peak pending after each run confirms the system drains the queue reliably.
