# Reliable Multi-Worker Judging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Redis consumption into horizontally scalable judge workers and make submission publication, ownership, retries, and result writes durable under duplicate delivery and worker crashes.

**Architecture:** The API atomically inserts a submission and PostgreSQL outbox event, while an API-side relay publishes events to Redis Streams. Worker slots consume the Redis group directly, acquire PostgreSQL leases with fencing tokens, heartbeat while judging in Docker, persist results with compare-and-set semantics, and acknowledge only after durable completion.

**Tech Stack:** Go 1.25, PostgreSQL 16 with pgx v5, Redis 7 Streams with go-redis v9, Docker sandbox, Docker Compose, Go testing, GitHub Actions.

---

## File Structure

New focused packages keep publishing and execution ownership separate:

```text
internal/domain/domain.go             shared outbox, lease, and claim result types
internal/store/memory.go              in-memory contract implementation
internal/store/postgres.go            PostgreSQL transaction and lease CAS operations
internal/store/reliability_test.go     memory store lease/outbox tests
internal/store/postgres_integration_test.go  real PostgreSQL concurrency tests
internal/outbox/relay.go               API-side durable publisher loop
internal/outbox/relay_test.go          relay retry and ownership tests
internal/queue/redis_streams.go        worker group dequeue, touch, retry, poison handling
internal/queue/redis_integration_test.go real Redis pending/recovery tests
internal/judge/service.go              return infrastructure failures separately from verdicts
internal/judgeworker/processor.go      one-message lease/heartbeat/judge/ACK state machine
internal/judgeworker/processor_test.go orchestration and fencing tests
internal/judgeworker/pool.go           concurrent worker slots and shutdown
internal/judgeworker/pool_test.go      concurrency and shutdown tests
cmd/api/main.go                        HTTP API plus outbox relay only
cmd/worker/main.go                     PostgreSQL/Redis worker pool bootstrap
migrations/003_reliable_workers.sql    lease and outbox schema
scripts/fault-test.sh                  multi-worker abrupt-failure acceptance test
```

Delete `internal/dispatcher/` and `internal/workerapi/` only after replacement tests pass.

### Task 1: Add Reliability Schema and Domain Contracts

**Files:**
- Modify: `internal/domain/domain.go`
- Create: `migrations/003_reliable_workers.sql`
- Test: `internal/domain/domain_test.go`

- [ ] **Step 1: Write failing domain tests**

Add tests that require explicit claim states and terminal-status detection:

```go
func TestIsTerminalSubmissionStatus(t *testing.T) {
    for _, status := range []domain.SubmissionStatus{
        domain.StatusAccepted, domain.StatusWrongAnswer,
        domain.StatusRuntimeError, domain.StatusTimeLimitExceeded,
        domain.StatusInternalError,
    } {
        if !domain.IsTerminalSubmissionStatus(status) {
            t.Fatalf("%q should be terminal", status)
        }
    }
    if domain.IsTerminalSubmissionStatus(domain.StatusRunning) {
        t.Fatal("running must not be terminal")
    }
}
```

- [ ] **Step 2: Verify the test fails**

Run: `go test ./internal/domain -run TestIsTerminalSubmissionStatus -v`

Expected: FAIL because `IsTerminalSubmissionStatus` is undefined.

- [ ] **Step 3: Add exact domain types**

Add:

```go
type ClaimState string

const (
    ClaimAcquired           ClaimState = "acquired"
    ClaimTerminal           ClaimState = "terminal"
    ClaimActiveSameReceipt  ClaimState = "active_same_receipt"
    ClaimActiveOtherReceipt ClaimState = "active_other_receipt"
    ClaimMissing            ClaimState = "missing"
)

type OutboxEvent struct {
    ID              int64
    SubmissionID    string
    ClaimToken      string
    PublishAttempts int
}

type SubmissionClaim struct {
    State         ClaimState
    Submission    Submission
    Token         string
    WorkerID      string
    Receipt       string
    ActiveReceipt string
    LeaseExpiresAt time.Time
    Attempts      int
}

func IsTerminalSubmissionStatus(status SubmissionStatus) bool {
    switch status {
    case StatusAccepted, StatusWrongAnswer, StatusRuntimeError,
        StatusTimeLimitExceeded, StatusInternalError:
        return true
    default:
        return false
    }
}
```

Extend `Job` with `OutboxID int64`; keep attempts and receipt internal to JSON.

- [ ] **Step 4: Add the migration**

Create repeatable DDL with these statements:

```sql
ALTER TABLE submissions
  ADD COLUMN IF NOT EXISTS judge_token TEXT,
  ADD COLUMN IF NOT EXISTS judge_worker_id TEXT,
  ADD COLUMN IF NOT EXISTS judge_receipt TEXT,
  ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS judge_attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_error TEXT;

CREATE INDEX IF NOT EXISTS submissions_judge_recovery_idx
  ON submissions (lease_expires_at, id)
  WHERE status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS judge_outbox (
  id BIGSERIAL PRIMARY KEY,
  submission_id TEXT NOT NULL UNIQUE REFERENCES submissions(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  claimed_by TEXT,
  claim_expires_at TIMESTAMPTZ,
  publish_attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error TEXT
);

CREATE INDEX IF NOT EXISTS judge_outbox_publish_idx
  ON judge_outbox (next_attempt_at, id)
  WHERE published_at IS NULL;
```

- [ ] **Step 5: Run focused tests and validate SQL formatting**

Run: `go test ./internal/domain && git diff --check`

Expected: PASS and no whitespace errors.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/domain.go internal/domain/domain_test.go migrations/003_reliable_workers.sql
git commit -m "feat: define reliable judge state"
```

### Task 2: Implement the Store Lease Contract In Memory

**Files:**
- Modify: `internal/store/memory.go`
- Create: `internal/store/reliability.go`
- Create: `internal/store/reliability_test.go`

- [ ] **Step 1: Write failing state-machine tests**

Cover these exact transitions using a fixed `now`:

```go
claim, err := st.ClaimSubmission(ctx, sub.ID, "worker-a", "token-a", "1-0", now, 30*time.Second)
if err != nil || claim.State != domain.ClaimAcquired || claim.Attempts != 1 {
    t.Fatalf("first claim = %+v, %v", claim, err)
}

other, _ := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "2-0", now.Add(time.Second), 30*time.Second)
if other.State != domain.ClaimActiveOtherReceipt { t.Fatalf("state = %q", other.State) }

same, _ := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "1-0", now.Add(time.Second), 30*time.Second)
if same.State != domain.ClaimActiveSameReceipt { t.Fatalf("state = %q", same.State) }

replacement, _ := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "1-0", now.Add(31*time.Second), 30*time.Second)
if replacement.State != domain.ClaimAcquired || replacement.Attempts != 2 { t.Fatalf("replacement = %+v", replacement) }
```

Also test stale renew, stale complete, completion after expiry, conditional release, terminal claim, and outbox creation alongside submissions.

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/store -run 'TestMemory(Claim|Lease|Outbox)' -v`

Expected: FAIL because the reliability methods do not exist.

- [ ] **Step 3: Define narrow store contracts**

In `reliability.go`, define:

```go
type LeaseStore interface {
    GetProblem(context.Context, string) (domain.Problem, bool, error)
    ClaimSubmission(context.Context, string, string, string, string, time.Time, time.Duration) (domain.SubmissionClaim, error)
    RenewSubmissionLease(context.Context, string, string, time.Time, time.Duration) (bool, error)
    CompleteSubmission(context.Context, string, string, time.Time, domain.JudgeResult) (bool, error)
    ReleaseSubmission(context.Context, string, string, time.Time, string) (bool, error)
}

type OutboxStore interface {
    ClaimOutbox(context.Context, string, time.Time, time.Duration, int) ([]domain.OutboxEvent, error)
    MarkOutboxPublished(context.Context, int64, string, time.Time) (bool, error)
    MarkOutboxFailed(context.Context, int64, string, time.Time, string) (bool, error)
}
```

- [ ] **Step 4: Implement memory state**

Add private maps for lease metadata and outbox rows. Keep token fields outside public `domain.Submission`. Implement every transition while holding the existing mutex. `CompleteSubmission` must require matching token, running status, and `now.Before(leaseExpiresAt)`.

- [ ] **Step 5: Run store and race tests**

Run: `go test -race ./internal/store`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/memory.go internal/store/reliability.go internal/store/reliability_test.go
git commit -m "feat: add submission lease state machine"
```

### Task 3: Implement PostgreSQL Transactions, Outbox Claims, and Fencing

**Files:**
- Modify: `internal/store/postgres.go`
- Create: `internal/store/postgres_integration_test.go`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Write PostgreSQL integration tests behind `integration` build tag**

Use `TEST_DATABASE_URL`, apply `001_init.sql`, `002_seed.sql`, and `003_reliable_workers.sql`, and verify:

```go
claims := make(chan domain.SubmissionClaim, 2)
go claim("worker-a", "token-a", "1-0", claims)
go claim("worker-b", "token-b", "2-0", claims)
// Assert exactly one ClaimAcquired and one active-lease state.
```

Add tests for atomic submission/outbox insertion, concurrent outbox claims, expired takeover, stale renewal, stale completion, and successful fenced completion.

- [ ] **Step 2: Run integration tests to verify failure**

Run: `TEST_DATABASE_URL=postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable go test -tags=integration ./internal/store -v`

Expected: FAIL because PostgreSQL reliability methods are missing.

- [ ] **Step 3: Make submission plus outbox atomic**

Change `PostgresStore.CreateSubmission` to begin a pgx transaction, insert the submission, insert one unique outbox row, and commit. Roll back on every error.

- [ ] **Step 4: Implement outbox claiming**

Use one CTE with `FOR UPDATE SKIP LOCKED` and an updating `RETURNING` clause:

```sql
WITH candidates AS (
  SELECT id FROM judge_outbox
  WHERE published_at IS NULL
    AND next_attempt_at <= $1
    AND (claimed_by IS NULL OR claim_expires_at <= $1)
  ORDER BY id
  FOR UPDATE SKIP LOCKED
  LIMIT $2
)
UPDATE judge_outbox o
SET claimed_by = $3, claim_expires_at = $4,
    publish_attempts = publish_attempts + 1
FROM candidates c
WHERE o.id = c.id
RETURNING o.id, o.submission_id, o.claimed_by;
```

Published/failed transitions must include `WHERE id = $1 AND claimed_by = $2 AND published_at IS NULL`.

- [ ] **Step 5: Implement transactional lease claims**

Use a row-locking `SELECT` with `FOR UPDATE`, classify the current row, then update only queued or expired-running rows before commit. Implement renew, release, and complete as conditional updates and return `RowsAffected() == 1`.

- [ ] **Step 6: Add CI service dependencies**

Add PostgreSQL 16 and Redis 7 service containers to the Go job, set `TEST_DATABASE_URL` and `TEST_REDIS_ADDR`, and run both `go test ./...` and `go test -tags=integration ./internal/store ./internal/queue`.

- [ ] **Step 7: Run unit and integration tests**

Run: `go test ./internal/store && TEST_DATABASE_URL=postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable go test -tags=integration ./internal/store -v`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/store/postgres.go internal/store/postgres_integration_test.go .github/workflows/ci.yml
git commit -m "feat: persist judge leases and outbox"
```

### Task 4: Add the Transactional Outbox Relay

**Files:**
- Create: `internal/outbox/relay.go`
- Create: `internal/outbox/relay_test.go`

- [ ] **Step 1: Write failing relay tests**

Use fake stores and publishers to prove claim, publish, mark-success ordering; publication failure marking; duplicate publication tolerance; cancellation; and bounded backoff:

```go
relay := outbox.New(store, publisher, outbox.Config{
    RelayID: "api-1", BatchSize: 10,
    ClaimDuration: 30*time.Second, PollInterval: time.Millisecond,
})
if err := relay.PublishBatch(ctx, now); err != nil { t.Fatal(err) }
if got := calls; !reflect.DeepEqual(got, []string{"claim", "publish", "published"}) {
    t.Fatalf("calls = %v", got)
}
```

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/outbox -v`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement relay configuration and batch publishing**

`PublishBatch` claims rows, publishes `domain.Job{SubmissionID: event.SubmissionID, OutboxID: event.ID}`, and conditionally marks each row. On publish failure, calculate `min(30s, 250ms * 2^(attempt-1))`, call `MarkOutboxFailed`, and continue remaining events while returning a joined error.

- [ ] **Step 4: Implement the polling loop**

`Run` invokes a batch immediately, logs transient errors, then polls until context cancellation. Cancellation returns `ctx.Err()` and must not be logged as a service failure by the caller.

- [ ] **Step 5: Run relay race tests**

Run: `go test -race ./internal/outbox`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/outbox
git commit -m "feat: publish judge jobs through outbox"
```

### Task 5: Harden Redis Streams for Worker Ownership

**Files:**
- Modify: `internal/queue/redis_streams.go`
- Modify: `internal/queue/memory.go`
- Modify: `internal/queue/redis_streams_test.go`
- Create: `internal/queue/redis_integration_test.go`

- [ ] **Step 1: Write failing mapping and poison-message tests**

Require `outbox_id` round trips, `Touch` resets pending idle time, explicit `Retry` and `DeadLetter` atomically add-and-ack, and malformed entries move to the dead stream before consumption continues.

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/queue -v`

Expected: FAIL because touch/dead-letter APIs and outbox mapping are absent.

- [ ] **Step 3: Replace the queue contract**

Implement these methods on Redis and memory queues:

```go
Dequeue(context.Context) (domain.Job, error)
Touch(context.Context, domain.Job) error
Ack(context.Context, domain.Job) error
Retry(context.Context, domain.Job, int, error) error
DeadLetter(context.Context, domain.Job, int, error) error
```

Keep `Enqueue` for outbox publication. Generate consumer names outside the queue constructor.

- [ ] **Step 4: Handle malformed messages inside dequeue**

When parsing fails, use one Redis transaction to `XADD` the original fields plus `parse_error` to the dead stream and `XACK` the poison receipt, then continue reading. Never return a parse error that leaves the same pending message in a tight loop.

- [ ] **Step 5: Add real Redis integration coverage**

Under the `integration` tag, verify two consumers distribute messages, `Touch` prevents early idle takeover, `XAUTOCLAIM` recovers an idle message, retry acknowledges the old ID and creates a new ID, and poison input appears in the dead stream.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/queue && TEST_REDIS_ADDR=localhost:16379 go test -tags=integration ./internal/queue -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/queue
git commit -m "feat: support reliable worker stream ownership"
```

### Task 6: Separate Judge Verdicts from Infrastructure Errors

**Files:**
- Modify: `internal/judge/service.go`
- Modify: `internal/judge/judge_test.go`
- Modify: `internal/judge/docker_runner_test.go`

- [ ] **Step 1: Write a failing infrastructure-error test**

```go
runner := &errorRunner{err: errors.New("docker daemon unavailable")}
result, err := judge.NewService(runner).Evaluate(ctx, problem, domain.LanguageGo, "code")
if err == nil || result.Status != "" { t.Fatalf("result=%+v err=%v", result, err) }
```

Keep existing timeout, non-zero exit, wrong-answer, and accepted tests as verdict tests. Add a parent-context cancellation test so shutdown or lease loss returns `context.Canceled` instead of being stored as a user time-limit verdict.

- [ ] **Step 2: Verify the new test fails**

Run: `go test ./internal/judge -run TestEvaluateReturnsRunnerInfrastructureError -v`

Expected: FAIL because `Evaluate` currently converts runner errors into verdicts.

- [ ] **Step 3: Change `Evaluate` to return `(JudgeResult, error)`**

Docker invocation failures return an error. `RunResult.TimedOut`, non-zero user exit, output mismatch, and successful execution remain normal judge results. In `DockerRunner.runPrepared`, return the parent `ctx.Err()` when the parent was cancelled, while retaining `TimedOut` for the child execution deadline. Update all call sites temporarily so the repository compiles.

- [ ] **Step 4: Run judge tests**

Run: `go test ./internal/judge -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/judge
git commit -m "refactor: separate judge infrastructure errors"
```

### Task 7: Build the Fenced Worker Processor

**Files:**
- Create: `internal/judgeworker/processor.go`
- Create: `internal/judgeworker/processor_test.go`

- [ ] **Step 1: Write orchestration tests with strict call ordering**

Test acquired lease, terminal ACK, different-receipt duplicate ACK, same-receipt no ACK, heartbeat lease loss, result-before-ACK, stale completion, retry, third-attempt dead letter, missing submission poison handling, and missing problem internal error.

The success assertion must be explicit:

```go
want := []string{"dequeue", "claim", "judge", "complete", "ack"}
if !reflect.DeepEqual(calls, want) { t.Fatalf("calls=%v want=%v", calls, want) }
```

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/judgeworker -run TestProcessor -v`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement `Processor.ProcessOne`**

Dependencies are `store.LeaseStore`, a local `WorkerQueue` interface, and:

```go
type Judge interface {
    Evaluate(context.Context, domain.Problem, domain.Language, string) (domain.JudgeResult, error)
}
```

Generate tokens with `crypto/rand`. Classify claim states exactly as specified. Start heartbeat only after acquisition. Complete through fenced CAS, stop heartbeat, then ACK.

- [ ] **Step 4: Implement heartbeat cancellation**

Every heartbeat interval, renew PostgreSQL first and then touch Redis. A false renewal cancels judging with `ErrLeaseLost`. Transient heartbeat errors are retried only while the existing lease has not expired; once authority cannot be proven before expiry, cancel without ACK.

- [ ] **Step 5: Implement retry/dead-letter decisions**

Use claim `Attempts` as authoritative. Below max, conditionally release then call queue retry. At max, conditionally complete `internal_error`, then dead-letter. If any database transition fails, leave the current message pending.

- [ ] **Step 6: Run worker processor race tests**

Run: `go test -race ./internal/judgeworker -run TestProcessor -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/judgeworker/processor.go internal/judgeworker/processor_test.go
git commit -m "feat: add fenced judge worker processor"
```

### Task 8: Add Worker Pool and Rewire Executables

**Files:**
- Create: `internal/judgeworker/pool.go`
- Create: `internal/judgeworker/pool_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/api/main.go`
- Modify: `cmd/worker/main.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Delete: `internal/dispatcher/`
- Delete: `internal/workerapi/`

- [ ] **Step 1: Write configuration and pool tests**

Require worker defaults, duration validation, unique slot consumer IDs, concurrent slot startup, stop-reading-on-cancel, and bounded shutdown. Add API configuration tests that reject exactly one of `DATABASE_URL`/`REDIS_ADDR`.

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/config ./internal/judgeworker -v`

Expected: FAIL because worker config and pool do not exist.

- [ ] **Step 3: Implement worker configuration**

Parse the exact environment variables in the design with `time.ParseDuration` and `strconv.Atoi`. Reject concurrency below one, heartbeat not shorter than lease, and missing worker database/Redis settings.

- [ ] **Step 4: Implement the pool**

Create one queue/processor per slot using consumer name `WORKER_ID-slot-N`. Run slots with an error group pattern, stop acquiring when context is cancelled, and wait up to `WORKER_SHUTDOWN_GRACE` before cancelling active judge contexts.

- [ ] **Step 5: Rewire worker main**

Open pgx and Redis clients, initialize the consumer group, construct `judge.Service` with the Docker runner, start the pool, and close dependencies. Do not create an HTTP server.

- [ ] **Step 6: Rewire API main and handler**

Remove the queue call from `POST /submissions`; PostgreSQL `CreateSubmission` now records the durable intent. Start `outbox.Relay` with the Redis publisher. Memory mode creates submissions but logs that no durable judge relay is active.

- [ ] **Step 7: Remove old dispatch packages**

Delete API dispatcher and worker HTTP code only after all replacement package tests pass. Remove `WORKER_URL` and `WORKER_ADDR` from configuration and tests.

- [ ] **Step 8: Run all Go tests and vet**

Run: `go test ./... && go vet ./...`

Expected: PASS with no old dispatcher/worker HTTP imports.

- [ ] **Step 9: Commit**

```bash
git add cmd internal
git commit -m "feat: run scalable workers from redis"
```

### Task 9: Update Compose, Migration Workflow, CI, and Fault Test

**Files:**
- Modify: `docker-compose.yml`
- Modify: `.env.example`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Create: `scripts/fault-test.sh`

- [ ] **Step 1: Write Compose/config assertions**

Add a shell test target that runs `docker compose config` and rejects `WORKER_URL`, `WORKER_ADDR`, or a worker `ports:` block. Assert worker contains PostgreSQL, Redis, Docker socket, and workdir settings.

- [ ] **Step 2: Update Compose**

API depends on PostgreSQL and Redis only. Worker receives `DATABASE_URL`, `REDIS_ADDR`, lease/concurrency variables, and depends on healthy PostgreSQL and Redis. Remove the worker HTTP port entirely.

- [ ] **Step 3: Add existing-volume migration target**

Add:

```make
migrate-reliable-workers:
	docker compose exec -T postgres psql -U codingjudge -d codingjudge -v ON_ERROR_STOP=1 -f /dev/stdin < migrations/003_reliable_workers.sql
```

Document that fresh volumes apply all migration files automatically and existing volumes run this target once.

- [ ] **Step 4: Add fault acceptance script**

The script starts the stack with two workers, submits a deliberately long-running solution, polls until `running`, identifies one worker container, sends `SIGKILL`, waits beyond the lease duration, then asserts the submission reaches a terminal state and Redis pending count returns to zero. Use `set -euo pipefail` and clean up only resources created by the script.

- [ ] **Step 5: Run Compose validation**

Run: `docker compose config --quiet && make test`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml .env.example Makefile .github/workflows/ci.yml scripts/fault-test.sh
git commit -m "build: configure reliable worker scaling"
```

### Task 10: Documentation, End-to-End Verification, and Review

**Files:**
- Modify: `README.md`
- Modify: `docs/development-plan.md`
- Modify: `docs/openapi.yaml` only if visible response details change

- [ ] **Step 1: Update documentation**

Replace the dispatcher architecture with direct worker consumption. Document outbox dual-write safety, lease/token semantics, retry/dead-letter behavior, scaling command, new environment variables, existing-volume migration, and fault-test procedure. State explicitly that delivery is at-least-once and effects are idempotent.

- [ ] **Step 2: Run focused verification**

```bash
GOCACHE=$PWD/.cache/go-build go test -race ./...
GOCACHE=$PWD/.cache/go-build go vet ./...
docker compose config --quiet
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend run test:run
npm --prefix frontend run build
git diff --check
```

Expected: every command exits zero.

- [ ] **Step 3: Run integration and Compose E2E**

```bash
make judge-images
docker compose up -d --build --scale worker=2
TEST_DATABASE_URL=postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable \
TEST_REDIS_ADDR=localhost:16379 \
go test -tags=integration ./internal/store ./internal/queue -v
npm --prefix frontend run test:e2e
bash scripts/fault-test.sh
```

Expected: integration tests pass, browser flows pass, killed work is recovered, and Redis reports no permanent pending entries.

- [ ] **Step 4: Request code review and fix actionable findings**

Review for lease race conditions, ACK-before-commit errors, outbox loss windows, shutdown leaks, SQL index use, public API regressions, and missing tests. Apply only findings grounded in the approved design.

- [ ] **Step 5: Commit final documentation**

```bash
git add README.md docs/development-plan.md docs/openapi.yaml
git commit -m "docs: explain reliable multi-worker judging"
```

- [ ] **Step 6: Verify clean final state**

Run: `git status --short && git log --oneline -12`

Expected: clean status and the task commits listed in order.
