ALTER TABLE submissions
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
    ADD COLUMN IF NOT EXISTS idempotency_request_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS submissions_user_idempotency_idx
    ON submissions (user_id, idempotency_key)
    WHERE user_id IS NOT NULL AND idempotency_key IS NOT NULL;

DO $$ BEGIN
    ALTER TABLE submissions ADD CONSTRAINT submissions_idempotency_pair_check
        CHECK ((idempotency_key IS NULL) = (idempotency_request_hash IS NULL));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submissions ADD CONSTRAINT submissions_idempotency_key_length_check
        CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 1 AND 128);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submissions ADD CONSTRAINT submissions_idempotency_hash_check
        CHECK (idempotency_request_hash IS NULL OR idempotency_request_hash ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
