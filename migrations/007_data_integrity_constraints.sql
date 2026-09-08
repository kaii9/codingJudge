DO $$ BEGIN
    ALTER TABLE problems ADD CONSTRAINT problems_language_check
        CHECK (language IN ('go', 'cpp', 'python'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE problems ADD CONSTRAINT problems_time_limit_check
        CHECK (time_limit_ms > 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE problems ADD CONSTRAINT problems_memory_limit_check
        CHECK (memory_limit_mb > 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submissions ADD CONSTRAINT submissions_language_check
        CHECK (language IN ('go', 'cpp', 'python'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submissions ADD CONSTRAINT submissions_status_check
        CHECK (status IN ('queued', 'running', 'accepted', 'wrong_answer', 'compile_error', 'runtime_error', 'time_limit_exceeded', 'internal_error'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submissions ADD CONSTRAINT submissions_judge_attempts_check
        CHECK (judge_attempts >= 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE judge_outbox ADD CONSTRAINT judge_outbox_publish_attempts_check
        CHECK (publish_attempts >= 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE problem_test_cases ADD CONSTRAINT problem_test_cases_sizes_check
        CHECK ((input_size_bytes IS NULL OR input_size_bytes >= 0)
           AND (expected_output_size_bytes IS NULL OR expected_output_size_bytes >= 0));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submission_artifacts ADD CONSTRAINT submission_artifacts_attempt_check
        CHECK (attempt > 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submission_artifacts ADD CONSTRAINT submission_artifacts_kind_check
        CHECK (kind IN ('source', 'stdout', 'stderr'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE submission_artifacts ADD CONSTRAINT submission_artifacts_size_check
        CHECK (size_bytes >= 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
