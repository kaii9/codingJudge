# GoJudge Microservices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Deliver the approved three-domain microservice topology with governed execution, data ownership and reproducible fault verification.

**Architecture:** Platform owns identity/catalog and delegates submissions over authenticated gRPC. Judge API and Worker own one judge database; Executor owns Docker execution. Outbox/Streams retain at-least-once semantics.

**Tech Stack:** Go 1.25, chi, gRPC/Protobuf, PostgreSQL/pgx, Redis Streams, MinIO, Docker Compose.

**Spec:** docs/superpowers/specs/2026-10-13-gojudge-microservices-design.md

## Global Constraints

- Keep public HTTP routes and verdict semantics compatible.
- Do not delete or overwrite current primary-workspace changes or database volumes.
- Preserve transactional outbox, database idempotency, leases and fencing.
- No automatic RunBatch replay inside RPC client.
- No merge/push without a user request.
- Work in feat/gojudge-microservices; implement inline.

## Review Focus

- Lost response after server execution must not become a user-code verdict.
- Empty or malformed batches must never invoke a Runner.
- Cancellation while waiting for capacity must release waiters without running.
- Idempotent replays after catalog updates must retain the first snapshot.
- Duplicate completion events must not increase solved count.

## Task 1: Govern executor protocol, admission and lifecycle

**Files:** api/proto/gojudge/executor/v1/executor.proto; internal/executor/{grpc_client,grpc_server,errors,admission}.go; internal/executor/*test.go; cmd/executor/main.go; internal/config/config.go.

**Interfaces:** Consumes judge.Runner/BatchRunner. Produces authenticated gRPC RunBatch, typed CallError, bounded admission, health-aware client and drain lifecycle.

- [x] Add cancellation and invalid-input regression tests around the real server. Example:
```go
func TestExecutorRejectsOversubscribedWaiters(t *testing.T) {
    // Hold one real RPC open; fill the one waiting slot.
    // A third call must return ResourceExhausted without invoking Runner.
}
```
- [x] Run focused tests and record RED output before implementation.
- [x] Define versioned protobuf, generate Go stubs, implement adapters and deadline/health/auth behavior.
- [x] Add a real two-server failover/recovery test and a lost-response test; prove no implicit execution replay.
- [x] Run `go test -race ./internal/executor ./internal/config ./cmd/executor`; commit governed executor slice.

## Task 2: Extract judge domain and immutable problem snapshots

**Files:** api/proto/gojudge/judge/v1/judge.proto; internal/judgerpc; internal/platform; internal/store; cmd/judge-api; cmd/api; cmd/worker; migrations/ and deploy/db.

**Interfaces:** Consumes executor protocol and existing store APIs. Produces Create/Get/List/Find RPCs and platform store adapter; snapshot lookup by submission ID.

- [x] Write integration test that creates a submission with a snapshot, updates catalog, then judges the first snapshot. Example:
```go
func TestSubmissionReplayKeepsOriginalSnapshot(t *testing.T) {
    // Create with v1, repeat same client fingerprint carrying v2.
    // Exactly one submission/outbox and stored snapshot remains v1.
}
```
- [x] Run RED, then implement snapshot/submission/outbox transaction and per-user RPC ownership.
- [x] Add platform adapter; preserve public Handler validation and response behavior.
- [x] Create separate migration sets and accounts; test unauthorized cross-domain SQL fails.
- [x] Run store/judgerpc/platform tests and full Go suite; commit judge-domain extraction.

## Task 3: Reliable completion events and leaderboard projection

**Files:** internal/store/{completion,projection}.go; internal/outbox; internal/queue; cmd/api; deploy migrations; domain event types.

**Interfaces:** Consumes final fenced completion. Produces SubmissionFinished outbox and idempotent platform projection.

- [x] Write duplicate-event and rollback tests. Example:
```go
func TestDuplicateAcceptedEventCountsOneSolvedProblem(t *testing.T) {
    // Apply identical event twice and two AC submissions for one problem.
    // solved=1, accepted submissions=2.
}
```
- [x] Run RED and implement result/event atomic transaction.
- [x] Implement separate Stream relay/consumer with per-event dedupe transaction and version checks.
- [x] Connect platform leaderboard to local projection; validate one user's multiple cases.
- [x] Run focused integration and whole suite; commit projection slice.

## Task 4: Independent deployment, tracing and fault acceptance

**Files:** Dockerfile; deploy/compose/microservices.yml; internal/telemetry; protocol fixture tests; scripts/verify-microservices.sh; docs.

**Interfaces:** Consumes domain processes and streams. Produces separately deployable targets and full-system evidence.

- [x] Write process-level shutdown and mixed-version contract tests.
- [x] Run RED; add await-drain lifecycle and independent image targets.
- [x] Propagate trace identifiers over RPC/outbox messages without sensitive payload logging.
- [x] Start isolated Compose project, run real Go/C++/Python jobs, ownership checks, kill/slow/drop-response/drain scenarios and projection checks.
- [x] Run `go test ./...`, race, vetted configuration and available frontend E2E. Record actual results and limitations; commit deployment slice.

## Completion

- [x] Perform fresh-context whole-branch review; resolve critical/important findings with regression tests.
- [x] Update README/development-plan with actual shipped capabilities and precise evidence.
- [x] Leave branch and workspace reviewable; do not merge or push.

Final source: a97ec5c. Full local evidence, review repairs and explicit non-production claims: [verification report](../../microservices-verification-2026-10-14.md). Primary main and its untracked architecture document were preserved.

