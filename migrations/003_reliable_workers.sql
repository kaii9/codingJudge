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
