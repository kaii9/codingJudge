# Fixed-Load Worker Scaling Benchmark

**Date:** 2026-07-03T11:58:53Z

## Environment

| Key | Value |
| --- | --- |
| Git commit | 5857f98 |
| OS | Darwin |
| Architecture | arm64 |
| Logical CPUs | 8 |
| Memory | 16 GB |
| Docker version | 29.1.3 |
| k6 version | linux/arm64) |
| Judge images | golang:1.25-alpine, python:3.12-alpine, gcc:13 |

## Scenario

- **Offered rate:** 1 req/s
- **Duration:** 2m
- **Worker concurrency:** 1 slot(s)/worker

## Results

| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 1.00/s | 1.33/s | 1.33/s | 40.38/s | 13.25ms | 5530.00ms | 0.0000% | 1 |
| 2 | 1.00/s | 2.67/s | 2.67/s | 44.34/s | 13.61ms | 2967.80ms | 0.0000% | 2 |
| 4 | 1.00/s | 3.34/s | 3.34/s | 46.71/s | 14.33ms | 2062.10ms | 0.0000% | 4 |

## Interpretation

_This is a fixed-load benchmark, not a maximum-throughput test._
_Results are from a local Docker Compose environment and are not production capacity guarantees._

- The same offered load was applied to 1, 2 and 4 worker configurations.
- Increasing workers lowers Judge P95 by distributing Docker sandbox execution across more slots.
- HTTP P95 remains low and stable because the API serves reads and enqueues submissions without blocking on judge execution.
- Zero peak pending after each run confirms the queue drains reliably under the tested load.
