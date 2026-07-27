ALTER TABLE problem_test_cases
    ALTER COLUMN input DROP NOT NULL,
    ALTER COLUMN expected_output DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS input_object_key TEXT,
    ADD COLUMN IF NOT EXISTS expected_output_object_key TEXT,
    ADD COLUMN IF NOT EXISTS input_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS expected_output_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS input_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS expected_output_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS hidden BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS problem_test_cases_object_key_idx
    ON problem_test_cases (problem_id, input_object_key)
    WHERE input_object_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS submission_artifacts (
    id BIGSERIAL PRIMARY KEY,
    submission_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL,
    fencing_token TEXT NOT NULL,
    kind TEXT NOT NULL,
    object_key TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (submission_id, attempt, fencing_token, kind)
);

CREATE INDEX IF NOT EXISTS submission_artifacts_submission_idx
    ON submission_artifacts (submission_id, id);
