# Fixed-Load Worker Scaling Benchmark

**Date:** 2026-07-03T13:48:33Z

## Environment

| Key | Value |
| --- | --- |
| Git commit | 1a51556 |
| OS | Darwin |
| Architecture | arm64 |
| Logical CPUs | 8 |
| Memory | 16 GB |
| Docker version | 29.1.3 |
| k6 version | 2.0.0+dirty |
| Judge images | golang:1.25-alpine, python:3.12-alpine, gcc:13 |

## Scenario

- **Offered rate:** 1 req/s
- **Duration:** 2m
- **Executor:** constant-arrival-rate
- **Pre-allocated VUs:** 20
- **Max VUs:** 30

## Results

| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | Failure rate | Peak pending (sampled) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 1.00/s | 1.00/s | 1.00/s | 8.83/s | 4.85ms | 739.40ms | 0.0000% | 1 |
| 2 | 1.00/s | 1.00/s | 0.99/s | 10.00/s | 7.96ms | 1587.00ms | 0.0000% | 2 |
| 4 | 1.00/s | 1.00/s | 1.00/s | 9.79/s | 9.97ms | 1474.00ms | 0.0000% | 2 |

## Interpretation

_This is a fixed-load benchmark, not a maximum-throughput test._
_Results are from a local Docker Compose environment and are not production capacity guarantees._
_Workers use Docker socket passthrough (Docker-outside-of-Docker), not nested Docker-in-Docker._

- The same offered load was applied to 1, 2, and 4 worker configurations using a constant-arrival-rate executor.
- This benchmark compares Judge P95, HTTP P95, failure rate, and peak sampled pending under identical load.
- It does NOT measure maximum throughput or claim linear scalability.
- Peak Pending values are sampled every 5 seconds during the run and represent the highest observed value.
- Pending returns to 0 after each round, confirming the queue drains under the tested load.
- This benchmark uses Python submissions only; Go and C++ require Linux native Docker for reliable compilation timing.
