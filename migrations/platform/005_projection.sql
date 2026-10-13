CREATE TABLE processed_completion_events(event_id TEXT PRIMARY KEY,processed_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE submission_projection(
 submission_id TEXT PRIMARY KEY,user_id TEXT NOT NULL,problem_id TEXT NOT NULL,
 status TEXT NOT NULL,version INTEGER NOT NULL CHECK(version>0),finished_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX submission_projection_accepted_idx ON submission_projection(user_id,problem_id,finished_at) WHERE status='accepted';
