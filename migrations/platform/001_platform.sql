CREATE TABLE problems (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL,
 language TEXT NOT NULL CHECK(language IN ('go','cpp','python')),
 time_limit_ms INTEGER NOT NULL CHECK(time_limit_ms>0),
 memory_limit_mb INTEGER NOT NULL CHECK(memory_limit_mb>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE problem_test_cases (
 id BIGSERIAL PRIMARY KEY,problem_id TEXT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
 input TEXT,expected_output TEXT,input_object_key TEXT,expected_output_object_key TEXT,
 input_sha256 TEXT,expected_output_sha256 TEXT,input_size_bytes BIGINT,expected_output_size_bytes BIGINT,
 hidden BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE users (
 id TEXT PRIMARY KEY,username TEXT NOT NULL,username_normalized TEXT NOT NULL UNIQUE,
 password_hash TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE user_sessions (
 token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX user_sessions_user_idx ON user_sessions(user_id,expires_at);
