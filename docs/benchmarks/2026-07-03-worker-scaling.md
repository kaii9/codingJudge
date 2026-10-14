# Fixed-Load Worker Scaling Benchmark

> 历史实验（2026-10-15整理）：数据、Date、commit、环境与局限原样保留。文件名日期不替代正文Date；本实验不是后来微服务/gRPC拓扑的性能证明，也不代表生产容量。引用数值以本报告为准，不沿用旧教材里未匹配的阶段性数字；资料分类见[文档入口](../README.md)。

**Date:** 2026-09-07T16:40:28Z

## Environment

| Key | Value |
| --- | --- |
| Git commit | 3699f85 |
| Git tree | dirty |
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

| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | HTTP failure | Logical failure | Peak pending (sampled) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 1.00/s | 1.00/s | 1.00/s | 9.50/s | 4.99ms | 824.00ms | 0.0000% | 0.0000% | 1 |
| 2 | 1.00/s | 1.00/s | 1.00/s | 9.37/s | 5.17ms | 827.05ms | 0.0000% | 0.0000% | 1 |
| 4 | 1.00/s | 1.00/s | 1.00/s | 9.12/s | 5.70ms | 826.10ms | 0.0000% | 0.0000% | 1 |

## Interpretation

_This is a fixed-load benchmark, not a maximum-throughput test._
_Results are from a local Docker Compose environment and are not production capacity guarantees._
_Workers use Docker socket passthrough (Docker-outside-of-Docker), not nested Docker-in-Docker._

- The same offered load was applied to 1, 2, and 4 worker configurations using a constant-arrival-rate executor.
- All rounds report zero HTTP failures, zero logical failures, and zero dropped iterations.
- Pending returns to 0 after each round, confirming the queue drains under the tested load.
- Peak Pending values are sampled every 5 seconds during the run and represent the highest observed value.
- This is NOT a saturation or maximum-throughput benchmark; it compares latency at fixed load.
- At this offered rate, one worker was not saturated; similar 1/2/4-worker P95 values demonstrate stability, not a scaling gain.
- This benchmark uses Python submissions only; Go and C++ require Linux native Docker for reliable timing.
