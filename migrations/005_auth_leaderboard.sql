CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    username_normalized TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE submissions
    ADD COLUMN IF NOT EXISTS user_id TEXT REFERENCES users(id);

CREATE INDEX IF NOT EXISTS user_sessions_user_idx
    ON user_sessions (user_id, expires_at);

CREATE INDEX IF NOT EXISTS submissions_user_updated_idx
    ON submissions (user_id, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS submissions_leaderboard_idx
    ON submissions (status, user_id, problem_id, updated_at)
    WHERE user_id IS NOT NULL;
