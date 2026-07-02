# Reliable Multi-Worker Judging Design

**Date:** 2026-07-02

## Summary

Replace the API-owned dispatcher and worker HTTP endpoint with Redis Streams consumers running inside each judge worker. Multiple workers will compete through one consumer group, use PostgreSQL leases and fencing tokens to prevent concurrent or stale result writes, and acknowledge Redis messages only after the result is durably stored.

Submission creation will also use a transactional outbox. A submission and its publish intent are committed in one PostgreSQL transaction, then an API-side relay publishes that intent to Redis. This closes the current failure window where PostgreSQL contains a queued submission but Redis never receives its job.

The API remains unable to execute user code. The outbox relay only publishes job identifiers; Docker access and judging remain confined to worker processes.

## Goals

- Let multiple worker processes consume the same Redis Stream safely.
- Prevent two workers from producing competing authoritative results for one submission.
- Recover work after a worker crashes or loses connectivity.
- Prevent stale workers from overwriting a newer worker's result.
- Ensure a committed submission is eventually published even when Redis is temporarily unavailable.
- Preserve delayed acknowledgement, bounded retries, dead-lettering, and pending recovery.
- Make the failure semantics demonstrable through automated integration and fault tests.

## Non-Goals

- User authentication, contests, leaderboards, rate limiting, or admission control.
- Prometheus, Grafana, distributed tracing, or performance benchmarking; these form the next phase.
- MinIO-backed test cases.
- Supporting MySQL in this change. The store boundary will remain testable, but PostgreSQL is the only implemented database.
- Exactly-once message delivery. The system provides at-least-once delivery with idempotent effects.

## Current Problems

The current API process consumes Redis jobs, marks submissions as running, and calls one worker over HTTP. This has four limitations:

1. `WORKER_URL` points to one worker endpoint, so Redis consumer-group scaling occurs in the API dispatcher rather than at the judge workers.
2. Submission status updates are unconditional. Duplicate delivery or pending recovery can let two executions overwrite each other.
3. A process crash between the PostgreSQL insert and Redis `XADD` leaves a submission queued forever with no job.
4. The worker HTTP layer adds a second dispatch protocol without adding a durable ownership boundary.

## Target Architecture

```text
Browser
  -> Next.js
  -> Go API
       -> PostgreSQL transaction: submissions + judge_outbox
       -> Outbox Relay -> Redis Stream

Redis Consumer Group
  -> Worker A -> PostgreSQL lease -> Docker sandbox -> fenced result write -> XACK
  -> Worker B -> PostgreSQL lease -> Docker sandbox -> fenced result write -> XACK
  -> Worker N -> PostgreSQL lease -> Docker sandbox -> fenced result write -> XACK
```

The API does not consume judge jobs and does not call a worker. Workers do not expose an HTTP port. Each worker connects directly to PostgreSQL, Redis, and the local Docker daemon.

## Data Model

### Submission lease fields

A new versioned migration adds these internal columns to `submissions`:

```sql
judge_token       TEXT
judge_worker_id   TEXT
judge_receipt     TEXT
lease_expires_at  TIMESTAMPTZ
judge_attempts    INTEGER NOT NULL DEFAULT 0
last_error        TEXT
```

`judge_token` is a cryptographically random opaque token generated for every successful claim. `judge_receipt` stores the Redis Stream message ID associated with the active lease. None of these fields is returned by the public API.

Add a partial recovery index over active submissions:

```sql
CREATE INDEX submissions_judge_recovery_idx
ON submissions (lease_expires_at, id)
WHERE status IN ('queued', 'running');
```

### Transactional outbox

Add a `judge_outbox` table:

```sql
id                BIGSERIAL PRIMARY KEY
submission_id     TEXT NOT NULL UNIQUE REFERENCES submissions(id) ON DELETE CASCADE
created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
published_at      TIMESTAMPTZ
claimed_by        TEXT
claim_expires_at  TIMESTAMPTZ
publish_attempts  INTEGER NOT NULL DEFAULT 0
next_attempt_at   TIMESTAMPTZ NOT NULL DEFAULT now()
last_error        TEXT
```

An index on unpublished, claimable rows supports relay polling. One outbox row represents the initial judge intent for one submission. Redis messages include both `submission_id` and `outbox_id`; `outbox_id` is diagnostic and does not provide exactly-once delivery.

## Submission Creation and Outbox Relay

The PostgreSQL `CreateSubmission` implementation will open one transaction and insert both the queued submission and its outbox row. The HTTP handler returns `202 Accepted` after that transaction commits. Redis availability is not part of the request's success path.

The API starts an outbox relay only when configured with both PostgreSQL and Redis. The relay:

1. Claims a bounded batch of unpublished rows whose `next_attempt_at` has arrived, using PostgreSQL locking and a 30-second claim lease.
2. Publishes each row to the judge Redis Stream.
3. Marks the row published only if the relay still owns its claim.
4. Records failures, advances `next_attempt_at` with bounded exponential backoff, and releases the claim.

Multiple API instances can run relays concurrently. Claims use `FOR UPDATE SKIP LOCKED` plus `claimed_by` and `claim_expires_at`, so one slow relay does not block all other rows.

A crash after `XADD` but before `published_at` can publish a duplicate message. This is intentional: the system favors non-loss over uniqueness, and worker leases make duplicate delivery safe.

When both `DATABASE_URL` and `REDIS_ADDR` are absent, the API retains memory mode for unit tests and read/write demonstrations, but submissions remain queued because no separate process can consume an in-memory queue. Supplying only one of the two production dependencies is a configuration error and fails fast.

## Worker Process Model

The worker becomes a pure background process and requires PostgreSQL and Redis configuration at startup.

Configuration:

```text
DATABASE_URL              required
REDIS_ADDR                required
WORKER_ID                 default: hostname-processID
WORKER_CONCURRENCY        default: 1
JUDGE_LEASE_DURATION      default: 30s
JUDGE_HEARTBEAT_INTERVAL  default: 10s
JUDGE_MAX_ATTEMPTS        default: 3
WORKER_SHUTDOWN_GRACE     default: 30s
JUDGE_WORKDIR             existing behavior
JUDGE_IMAGE               existing optional override
```

Startup rejects non-positive durations and any heartbeat interval that is not strictly shorter than the lease duration.

Each concurrency slot uses a distinct Redis consumer name derived from `WORKER_ID` and its slot number. The default stays at one because each judge can consume a full CPU and significant memory. Horizontal scaling uses multiple worker containers; per-process concurrency is available for larger hosts.

The worker calls the existing judge service directly. The worker HTTP server, `internal/workerapi`, HTTP judge client, API dispatcher startup, and `WORKER_URL` configuration are removed.

## Lease and Fencing State Machine

### Claim

For each Redis message, a worker generates a new token and performs one conditional PostgreSQL update. A claim succeeds only when the submission is:

- `queued`, or
- `running` with `lease_expires_at` in the past.

The successful update sets `running`, the token, worker ID, Redis receipt, a new lease expiry, clears `last_error`, and increments `judge_attempts`. The updated submission and authoritative attempt count are returned to the worker.

Only one concurrent claimant can affect a row. A terminal submission is never claimable.

### Duplicate and reclaimed receipts

If a claim fails because another unexpired lease exists:

- A different Redis receipt for the same submission is a duplicate publication and may be acknowledged.
- The same receipt means the message was reclaimed while the original lease is still active. It must not be acknowledged. The current owner may reclaim/touch the receipt again, or recovery can continue after the lease expires.

Saving `judge_receipt` in PostgreSQL makes these cases distinguishable.

### Heartbeat

While Docker judging is active, the worker renews its PostgreSQL lease every 10 seconds using `submission_id + judge_token + running status`. It also touches the Redis pending receipt to reset its idle time.

Failure to renew before expiry means the worker has lost authority. Its judge context is cancelled, and it must not write a result or acknowledge the message.

### Complete

The final result update is a compare-and-set operation requiring:

- matching submission ID,
- `status = running`,
- matching `judge_token`, and
- an unexpired lease.

Success writes the terminal result and clears token, worker ID, receipt, and lease. Redis `XACK` happens only after this commit succeeds.

If the process crashes after the result commit but before `XACK`, a later consumer sees the terminal status and only acknowledges the message. It does not judge again.

### Retry and dead letter

Judge verdicts caused by submitted code (`accepted`, `wrong_answer`, `runtime_error`, and `time_limit_exceeded`) are terminal and are never infrastructure retries.

PostgreSQL, Redis, Docker startup, or worker-internal failures are infrastructure errors:

1. Below the attempt limit, the worker conditionally releases its lease back to `queued`, records `last_error`, then atomically adds a retry Stream entry and acknowledges the old entry in Redis.
2. If the process crashes between lease release and Redis retry, the old pending entry remains recoverable. PostgreSQL's `judge_attempts`, not the Redis payload, is authoritative for the limit.
3. At the third infrastructure processing attempt, the worker conditionally writes `internal_error`, publishes a dead-letter record, and acknowledges the current entry.

Malformed Redis messages are poison messages. The queue implementation sends them to the dead-letter stream with parse details and acknowledges them so they cannot cause a permanent consume loop.

## Redis Pending Recovery

Workers first inspect claimable pending messages and then block for new group messages. `XAUTOCLAIM` moves sufficiently idle entries to the current consumer. Active jobs normally remain non-idle because of heartbeat touches.

Redis message ownership is not treated as final authority. PostgreSQL's token and lease determine who may write. Redis provides work distribution and redelivery; PostgreSQL provides effect-level concurrency control.

## Graceful and Abrupt Shutdown

On `SIGTERM`, the worker stops reading new messages and gives active slots a configurable grace period to finish. Finished slots persist and acknowledge normally. At grace expiry, judge contexts are cancelled and messages remain unacknowledged for recovery.

On `SIGKILL` or host failure, leases expire and pending entries become claimable by another worker. A detached sandbox container may continue until its existing execution limit, but its old worker can no longer commit after the fencing token changes.

## Compose and Configuration Changes

- Remove `WORKER_URL` and `WORKER_ADDR`.
- Give the worker `DATABASE_URL` and `REDIS_ADDR`.
- Remove the worker HTTP port and API dependency on worker startup.
- Make worker depend on healthy PostgreSQL and Redis.
- Keep the Docker socket and shared work directory mounts.
- Document horizontal scaling with `docker compose up -d --scale worker=3`.
- Add a migration command for existing PostgreSQL volumes; do not require deleting persisted data.

## Interfaces and Package Boundaries

Introduce narrow interfaces around responsibilities:

- `OutboxStore`: claim, publish-success, and publish-failure transitions.
- `JobPublisher`: publish a durable outbox event to Redis.
- `LeaseStore`: claim, inspect, renew, release, and fenced completion.
- `WorkerQueue`: dequeue/reclaim, touch, acknowledge, retry, and dead-letter.
- `Judge`: execute one already-loaded submission/problem pair.

The worker orchestration belongs in a new worker-focused package rather than reusing `dispatcher`, because it owns leases, heartbeats, direct judging, and process concurrency. PostgreSQL and memory implementations receive contract tests for state transitions. The public HTTP store remains unaware of judge lease internals.

## Testing Strategy

### Unit tests

- Submission and outbox are created as one logical operation.
- Relay retries failed publication and tolerates duplicate publication.
- Two claims for one queued submission yield exactly one lease owner.
- An active lease rejects another worker.
- An expired lease can be replaced with a new token.
- Renew and complete reject stale tokens.
- Complete rejects an expired lease.
- Different-receipt duplicate messages are acknowledged without judging.
- Same-receipt premature reclaim is not acknowledged.
- Result persistence precedes acknowledgement.
- Result-persisted/ACK-missed recovery only acknowledges.
- Infrastructure errors retry below the limit and dead-letter at the limit.
- Malformed messages are dead-lettered instead of looping.
- Shutdown stops new claims and leaves unfinished work recoverable.

### PostgreSQL and Redis integration tests

CI starts real PostgreSQL and Redis services and verifies:

- transactional creation and outbox rows,
- concurrent `FOR UPDATE SKIP LOCKED` outbox claims,
- concurrent lease compare-and-set behavior,
- expired lease takeover and stale completion rejection,
- Redis group consumption, touch, acknowledgement, retry, and `XAUTOCLAIM`,
- duplicate outbox publication is effect-idempotent.

Integration tests use an explicit build tag or command and isolated test data so ordinary unit tests remain fast.

### Compose fault test

A scripted acceptance test will:

1. Start the full stack with at least two workers.
2. Submit a batch of Go, C++, and Python solutions.
3. Wait until one selected submission is running.
4. Kill its worker abruptly.
5. Wait for lease expiry and takeover.
6. Assert every submission reaches one terminal result, no queued item is orphaned, and Redis has no unexpected permanent pending entries.

## Acceptance Criteria

- The API contains no judge consumer, worker HTTP client, or code execution path.
- The worker exposes no HTTP judge endpoint and directly consumes Redis jobs.
- At least two worker containers can judge concurrently from one consumer group.
- A committed submission is eventually published after Redis recovers.
- Duplicate Stream entries cannot create competing terminal writes.
- A killed worker's submission is completed by another worker after lease expiry.
- A stale token cannot renew or complete a submission.
- Retry, dead-letter, and pending recovery behavior is covered by tests.
- Existing frontend API contracts and visible submission statuses remain unchanged.
- Go tests, race tests, vet, frontend tests/build, integration tests, Compose E2E, and documentation checks pass.

## Interview Narrative

The finished implementation demonstrates three separate reliability problems and their solutions:

1. **Cross-system dual write:** PostgreSQL transactional outbox prevents silent loss before Redis publication.
2. **At-least-once delivery:** Redis Streams can duplicate or redeliver, so effects are made idempotent rather than claiming exactly-once transport.
3. **Distributed ownership:** PostgreSQL leases recover crashed work, while fencing tokens reject stale workers and late results.

This keeps the security boundary explicit: API accepts and publishes work; isolated workers own Docker execution; PostgreSQL arbitrates durable ownership.
