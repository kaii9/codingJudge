# Worker Scaling Benchmark

**Date:** 2026-07-03T10:25:03Z | **Commit:** d19fb70 | **OS:** Darwin | **Arch:** arm64 | **CPUs:** 8

## Results

| Workers | Submission rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |
| --- | --- | --- | --- | --- | --- |
| 1 | 45.08/s | 5.38ms | 2356.20ms | 0.0000% | 1 |
| 2 | 48.82/s | 8.54ms | 1771.85ms | 0.0000% | 2 |
| 4 | 52.98/s | 6.35ms | 1041.70ms | 0.0000% | 4 |

## Interpretation

_This benchmark was run in a local Docker Compose environment. Results are not production capacity guarantees._

- CPU and memory contention increase as workers scale, but throughput should improve roughly linearly up to the number of logical CPUs.
- Judge P95 is dominated by Docker container startup and compilation time; it improves with more workers distributing the load.
- Zero or near-zero peak pending after each run confirms the system drains the queue reliably.
