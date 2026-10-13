ALTER TABLE judge_outbox ADD COLUMN trace_parent TEXT NOT NULL DEFAULT '';
ALTER TABLE completion_outbox ADD COLUMN trace_parent TEXT NOT NULL DEFAULT '';
