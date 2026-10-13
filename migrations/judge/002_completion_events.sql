CREATE TABLE completion_outbox(
 id BIGSERIAL PRIMARY KEY,event_id TEXT NOT NULL UNIQUE,
 submission_id TEXT NOT NULL REFERENCES submissions(id),payload JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),published_at TIMESTAMPTZ,
 claimed_by TEXT,claim_expires_at TIMESTAMPTZ,publish_attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),last_error TEXT
);
CREATE INDEX completion_outbox_publish_idx ON completion_outbox(next_attempt_at,id) WHERE published_at IS NULL;
