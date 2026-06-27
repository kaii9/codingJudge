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
- Dispatcher sends queued jobs to worker HTTP API.
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

- Multiple dispatcher consumers can share the stream group without duplicate normal processing.
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

Goal: provide a complete product demo without hiding backend complexity.

Tasks:

- Add Next.js app.
- Add problem list and problem detail pages.
- Add Monaco Editor submission page.
- Add submission status polling.
- Add submission history.

Acceptance:

- A reviewer can run Compose and complete the full user flow from browser.

## Phase 6: Product Extensions

Goal: turn MVP into a richer judge platform.

Tasks:

- User login.
- Admin problem management.
- Contest model.
- Leaderboard.
- MinIO test case storage.
- Prometheus metrics.

Acceptance:

- Project can be presented as a backend-heavy platform with production-aware architecture.

## Current Development Slice

The backend MVP slice is complete. The next optional slice is the browser demo in Phase 5.

Reason:

- The API, PostgreSQL persistence, Redis reliability and Docker sandbox have been exercised together through Compose.
- Go, C++ and Python accepted submissions have passed end-to-end.
- Wrong answer, runtime error, timeout and dead-letter paths have been exercised end-to-end.
- Frontend work can now consume a stable API without moving code execution into the API process.
