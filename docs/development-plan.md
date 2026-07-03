# GoJudge Development Plan

## Guiding Principle

先跑通安全判题主链路，再扩展持久化、队列、前端和比赛功能。每个阶段都必须保持 API 与 worker 隔离，避免 API 服务执行用户代码。

## Phase 0: Current MVP Foundation

Status: implemented.

- Go module and package layout.
- `cmd/api` and `cmd/worker` separated processes.
- In-memory problem/submission store.
- In-memory judge queue.
- Optional PostgreSQL store through `DATABASE_URL`.
- Optional Redis Streams queue through `REDIS_ADDR`.
- API endpoints for health, problems, submission creation and submission detail.
- API stores submissions and durable outbox intents; workers consume Redis directly.
- Worker evaluates submissions through Docker sandbox.
- Dockerfile, Docker Compose, Makefile, GitHub Actions.
- README, OpenAPI draft, PostgreSQL migration draft.

Verification:

```bash
make test
docker compose config
go build ./cmd/api ./cmd/worker
```

## Phase 1: MVP API Completion

Status: implemented.

Goal: make the current backend usable by a minimal frontend and easy to demo.

Tasks:

- Add `GET /submissions` for submission history.
- Keep submission responses code-free by default.
- Expand OpenAPI schemas for problems, submissions and judge results.
- Add consistent JSON error response structure.
- Add request size limits for code submission.
- Add basic structured access logs.
- Add Go/C++/Python language runtime specs in Docker runner.

Acceptance:

- Users can list problems, submit code, poll one submission and list submission history.
- All API behavior is covered by `httptest`.

## Phase 2: PostgreSQL Persistence

Status: implemented.

Goal: replace in-memory store with durable storage in Compose and production.

Tasks:

- Add migration files for problems, test cases, submissions and results.
- Implement PostgreSQL store behind the same store interface.
- Add seed data for sample problems.
- Extend Compose with PostgreSQL.

Acceptance:

- API can run with PostgreSQL store.
- Tests cover repository behavior with integration tests or testcontainers where available.

## Phase 3: Redis Streams Queue

Status: implemented.

Goal: replace in-memory queue with durable async judge pipeline.

Tasks:

- Add Redis service to Compose.
- Implement `XADD` for submission jobs.
- Implement consumer group using `XREADGROUP`.
- Add retry and dead-letter stream behavior.
- Add pending message recovery strategy.

Acceptance:

- Multiple judge workers can share the stream group without competing result writes.
- Failed jobs can be retried and eventually dead-lettered.

## Phase 4: Sandbox Hardening

Status: implemented for the MVP threat model.

Goal: make untrusted code execution visibly safer.

Tasks:

- Add per-language runner config.
- Harden C++ and Python support with language-specific compile/run reporting.
- Add compile step separation from run step.
- Add max output size.
- Add explicit timeout kill handling.
- Document Docker socket risk and future gVisor/Firecracker path.

Acceptance:

- Go, C++ and Python sample submissions can be judged.
- Infinite loop submissions become `time_limit_exceeded`.
- Non-zero exits become `runtime_error`.

## Phase 5: Frontend Demo

Status: implemented.

Goal: provide a complete product demo without hiding backend complexity.

Tasks:

- Add Next.js app.
- Add problem list and problem detail pages.
- Add Monaco Editor submission page.
- Add submission status polling.
- Add submission history.

Acceptance:

- A reviewer can run Compose and complete the full user flow from browser.
- Playwright covers Go, C++ and Python accepted submissions plus desktop/mobile layout behavior.

## Phase 6: Reliable Multi-Worker Judging

Status: implemented.

Goal: make publication and worker ownership recoverable under duplicate delivery and crashes.

Tasks:

- Add a PostgreSQL transactional outbox for submission publication.
- Move Redis Consumer Group ownership from the API to judge workers.
- Add PostgreSQL leases, heartbeats and fencing tokens.
- Support multiple worker processes and per-process concurrency.
- Reject stale result writes and recover idle Pending messages.
- Add real PostgreSQL/Redis integration tests and a Compose fault test.

Acceptance:

- Committed submissions are eventually published after Redis recovery.
- Duplicate jobs cannot create competing terminal results.
- A killed worker's job is reclaimed after lease expiry.
- API contains no user-code execution or judge-consumer path.

## Phase 7: Curated Problem Library

Status: implemented.

Goal: turn the sample-only catalog into a credible interview practice library.

Tasks:

- Add 20 original interview-style problems across arrays, linked lists, trees, graphs and dynamic programming.
- Keep `sum` and `echo` as a separate Starter collection.
- Normalize difficulty, collection order and topic tags in PostgreSQL.
- Seed at least six deterministic hidden cases per curated problem through an idempotent migration.
- Add collection, title/tag and difficulty filtering to the workbench.

Acceptance:

- PostgreSQL reports exactly 20 Hot problems and 2 Starter problems.
- Every Hot problem has two-to-four tags and at least six hidden cases.
- Existing database volumes can apply `make migrate-hot20` repeatedly without duplicates.
- The frontend can combine collection, search and difficulty filters without horizontal overflow.

## Phase 8: Observability and Load Testing

Status: implemented. Fixed-load benchmark: 1→4 workers scales accepted/s from 1.33→3.34, Judge P95 from 5530→2062ms.

Goal: add application-level Prometheus metrics, a provisioned Grafana dashboard, and reproducible k6 benchmarks comparing one, two, and four judge workers.

Tasks:

- Add 16 codingjudge_-prefixed application metrics with bounded labels.
- Instrument HTTP, submission creation, outbox, Redis queue, worker lifecycle and judge cases.
- Serve worker metrics on port 9091 and sample Redis Pending gauge from the API.
- Add Prometheus Compose service with DNS-based worker discovery and recording rules.
- Provision a Grafana dashboard with API, Queue/Outbox, Worker and Judge rows.
- Create k6 workloads (smoke, baseline, submissions, mixed) with safe cleanup.
- Automate 1/2/4 worker scaling benchmark and publish structured reports.

Acceptance:

- API and every scaled worker expose Prometheus metrics without high-cardinality labels.
- Prometheus discovers worker targets without configuration edits.
- Grafana starts with a usable dashboard requiring no manual setup.
- API and judging remain functional while Prometheus and Grafana are stopped.
- k6 smoke and baseline thresholds pass.
- A reproducible 1/2/4 worker report contains machine metadata and measured values.

## Phase 9: Product Extensions

Goal: turn MVP into a richer judge platform.

Tasks:

- User login.
- Admin problem management.
- Contest model.
- Leaderboard.
- MinIO test case storage.

Acceptance:

- Project can be presented as a backend-heavy platform with production-aware architecture.

## Current Development Slice

The backend MVP, browser demo, reliable multi-worker phase, curated problem library, and observability/load testing are complete. The next recommended slice is authentication and product extensions.

Reason:

- The API, PostgreSQL persistence, Redis reliability and Docker sandbox have been exercised together through Compose.
- PostgreSQL outbox, leases and fencing tokens protect dual writes and stale workers.
- Redis consumption runs directly in horizontally scalable judge workers.
- Go, C++ and Python accepted submissions have passed end-to-end.
- Wrong answer, runtime error, timeout and dead-letter paths have been exercised end-to-end.
- The Next.js workbench supports problem navigation, Monaco editing, status polling and submission history.
- The catalog contains 20 curated interview problems plus 2 Starter problems with normalized metadata and hidden cases.
- Collection, title/tag and difficulty filters keep the larger catalog navigable.
- Playwright verifies browser submissions and responsive desktop/mobile layouts.
- Prometheus and Grafana provide real-time observability with a pre-provisioned dashboard.
- k6 workloads and automated scaling benchmarks measure 45→53 req/s throughput with P95 judge latency improving from 2356ms (1 worker) to 1042ms (4 workers).

Not yet implemented:

- Authentication and user accounts.
- Contests, leaderboards and administration.
- MinIO-backed test-case storage.
