CREATE TABLE submissions (
 id TEXT PRIMARY KEY,user_id TEXT NOT NULL,problem_id TEXT NOT NULL,
 language TEXT NOT NULL CHECK(language IN ('go','cpp','python')),code TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','running','accepted','wrong_answer','compile_error','runtime_error','time_limit_exceeded','internal_error')),
 stdout TEXT,stderr TEXT,exit_code INTEGER,duration_ms BIGINT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 judge_token TEXT,judge_worker_id TEXT,judge_receipt TEXT,lease_expires_at TIMESTAMPTZ,
 judge_attempts INTEGER NOT NULL DEFAULT 0 CHECK(judge_attempts>=0),last_error TEXT,
 idempotency_key TEXT,idempotency_request_hash TEXT,
 CHECK((idempotency_key IS NULL)=(idempotency_request_hash IS NULL)),
 CHECK(idempotency_key IS NULL OR length(idempotency_key) BETWEEN 1 AND 128),
 CHECK(idempotency_request_hash IS NULL OR idempotency_request_hash~'^[0-9a-f]{64}$')
);
CREATE UNIQUE INDEX submissions_user_idempotency_idx ON submissions(user_id,idempotency_key)
 WHERE user_id IS NOT NULL AND idempotency_key IS NOT NULL;
CREATE INDEX submissions_user_updated_idx ON submissions(user_id,updated_at DESC,id DESC);
CREATE INDEX submissions_judge_recovery_idx ON submissions(lease_expires_at,id) WHERE status IN ('queued','running');
CREATE TABLE judge_outbox (
 id BIGSERIAL PRIMARY KEY,submission_id TEXT NOT NULL UNIQUE REFERENCES submissions(id) ON DELETE CASCADE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),published_at TIMESTAMPTZ,claimed_by TEXT,
 claim_expires_at TIMESTAMPTZ,publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK(publish_attempts>=0),
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),last_error TEXT
);
CREATE INDEX judge_outbox_publish_idx ON judge_outbox(next_attempt_at,id) WHERE published_at IS NULL;
CREATE TABLE submission_problem_snapshots (
 submission_id TEXT PRIMARY KEY REFERENCES submissions(id) ON DELETE CASCADE,
 snapshot JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE submission_artifacts (
 id BIGSERIAL PRIMARY KEY,submission_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
 attempt INTEGER NOT NULL CHECK(attempt>0),fencing_token TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('source','stdout','stderr')),object_key TEXT NOT NULL,
 sha256 TEXT NOT NULL,size_bytes BIGINT NOT NULL CHECK(size_bytes>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(submission_id,attempt,fencing_token,kind)
);
CREATE INDEX submission_artifacts_submission_idx ON submission_artifacts(submission_id,id);

