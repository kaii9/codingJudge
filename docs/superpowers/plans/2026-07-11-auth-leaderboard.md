# Auth and Leaderboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add user registration/login/logout, require login for code submission, bind submissions to users, and expose a solved-problem leaderboard.

**Architecture:** The Go API will use server-side sessions stored in PostgreSQL or the memory store. The browser receives an `HttpOnly` cookie containing only a random session token; the store persists a SHA-256 hash of that token. Leaderboard rows are derived from accepted submissions grouped by user, keeping the judge worker unchanged.

**Tech Stack:** Go `net/http` + chi, PostgreSQL + pgx, in-memory store for local tests, bcrypt from `golang.org/x/crypto/bcrypt`, Next.js API proxy, React client state.

## Global Constraints

- Do not execute user code in the API service.
- `POST /submissions`, `GET /submissions`, and `GET /submissions/{id}` require login after this change.
- Existing worker, outbox, Redis Streams, lease, and fencing-token behavior must remain unchanged.
- Store only password hashes, never plaintext passwords.
- Store only session token hashes, never raw session tokens.
- Use tests-first implementation for backend behavior and frontend API/proxy changes.
- Keep contests out of scope.

---

## File Structure

- `internal/auth/auth.go`: password hashing, password verification, random session token generation, token hashing.
- `internal/auth/auth_test.go`: unit tests for password and token helpers.
- `internal/domain/domain.go`: add `User`, `Session`, `LeaderboardEntry`, and `Submission.UserID`.
- `internal/store/memory.go`: add user/session methods, user-scoped submission listing, and leaderboard aggregation.
- `internal/store/postgres.go`: add user/session methods, authenticated submission creation, user-scoped queries, leaderboard query, and updated submission scanner.
- `migrations/005_auth_leaderboard.sql`: create `users`, `user_sessions`, add nullable `submissions.user_id`, add indexes.
- `internal/httpapi/server.go`: add `/auth/register`, `/auth/login`, `/auth/logout`, `/auth/me`, `/leaderboard`, auth middleware helpers, and protected submission routes.
- `internal/httpapi/server_test.go`: API tests for auth, protected submissions, and leaderboard.
- `frontend/app/api/[...path]/route.ts`: forward cookies and `Set-Cookie` across the same-origin proxy.
- `frontend/lib/types.ts` and `frontend/lib/api.ts`: add auth and leaderboard types/client functions.
- `frontend/components/app-shell.tsx`: show login/register/logout state and leaderboard navigation.
- `frontend/app/leaderboard/page.tsx`: render leaderboard.
- `frontend/tests/*.test.tsx` and `frontend/tests/proxy.test.ts`: cover proxy cookies and key UI states.
- `docs/codingjudge-interview-handbook.md`: add JWT vs Cookie Session interview QA.
- `README.md` and `docs/openapi.yaml`: document auth and leaderboard APIs.

## Task 1: Auth Helper Package

**Files:**
- Create: `internal/auth/auth.go`
- Create: `internal/auth/auth_test.go`
- Modify: `go.mod`

**Interfaces:**
- Produces: `HashPassword(password string) (string, error)`
- Produces: `CheckPassword(hash, password string) bool`
- Produces: `NewSessionToken() (string, error)`
- Produces: `HashSessionToken(token string) string`

- [ ] **Step 1: Write failing tests**

```go
func TestPasswordHashDoesNotStorePlaintextAndVerifies(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password hash must not equal plaintext")
	}
	if !auth.CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if auth.CheckPassword(hash, "wrong password") {
		t.Fatal("wrong password should not verify")
	}
}

func TestSessionTokenHashIsStableAndDoesNotExposeToken(t *testing.T) {
	token, err := auth.NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken returned error: %v", err)
	}
	first := auth.HashSessionToken(token)
	second := auth.HashSessionToken(token)
	if first == "" || first != second {
		t.Fatalf("hash should be stable, got %q and %q", first, second)
	}
	if first == token {
		t.Fatal("stored session hash must not equal raw token")
	}
}
```

- [ ] **Step 2: Run test to verify RED**

Run: `go test ./internal/auth`

Expected: FAIL because `internal/auth` does not exist.

- [ ] **Step 3: Implement helpers**

Use bcrypt cost `bcrypt.DefaultCost`, reject empty passwords in caller-level validation, generate 32 random bytes, base64 URL encode them, and hash tokens with SHA-256 hex.

- [ ] **Step 4: Run test to verify GREEN**

Run: `go test ./internal/auth`

Expected: PASS.

## Task 2: Domain and Store Auth Model

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `internal/store/memory.go`
- Modify: `internal/store/memory_test.go`
- Modify: `internal/store/postgres.go`
- Modify: `internal/store/postgres_test.go`
- Add: `migrations/005_auth_leaderboard.sql`

**Interfaces:**
- Produces: `CreateUser(ctx, username, passwordHash string) (domain.User, error)`
- Produces: `GetUserByUsername(ctx, username string) (domain.User, bool, error)`
- Produces: `CreateSession(ctx, userID, tokenHash string, expiresAt time.Time) (domain.Session, error)`
- Produces: `GetUserBySessionTokenHash(ctx, tokenHash string, now time.Time) (domain.User, bool, error)`
- Produces: `DeleteSession(ctx, tokenHash string) error`
- Produces: `ListSubmissionsByUser(ctx, userID string) ([]domain.Submission, error)`
- Produces: `GetSubmissionForUser(ctx, id, userID string) (domain.Submission, bool, error)`
- Produces: `ListLeaderboard(ctx, limit int) ([]domain.LeaderboardEntry, error)`

- [ ] **Step 1: Write failing store tests**

Add memory-store tests proving:
- duplicate usernames are rejected case-insensitively;
- session lookup returns the user before expiry and misses after expiry;
- user-scoped submission list excludes other users;
- leaderboard counts accepted unique problems per user.

- [ ] **Step 2: Run test to verify RED**

Run: `go test ./internal/store`

Expected: FAIL because the new methods and domain types do not exist.

- [ ] **Step 3: Implement domain and memory store**

Add domain types, normalize usernames to lowercase for uniqueness, generate user IDs with existing in-memory counters, keep old submissions with empty `UserID` inaccessible through user-scoped HTTP methods, and aggregate accepted submissions by unique `(user_id, problem_id)`.

- [ ] **Step 4: Implement PostgreSQL schema and queries**

Create tables and indexes:

```sql
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
```

- [ ] **Step 5: Run store tests**

Run: `go test ./internal/store`

Expected: PASS.

## Task 3: HTTP Auth and Protected Submissions

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes store methods from Task 2 and auth helpers from Task 1.
- Produces API endpoints: `POST /auth/register`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `GET /leaderboard`.

- [ ] **Step 1: Write failing HTTP tests**

Add tests for:
- register sets an `HttpOnly` session cookie and `/auth/me` returns the user;
- login rejects wrong password with `401`;
- logout clears session;
- anonymous `POST /submissions` returns `401`;
- authenticated `POST /submissions` persists `UserID`;
- authenticated `GET /submissions` returns only the current user's submissions;
- `GET /leaderboard` returns accepted counts.

- [ ] **Step 2: Run test to verify RED**

Run: `go test ./internal/httpapi`

Expected: FAIL because auth routes and protected behavior are missing.

- [ ] **Step 3: Implement HTTP handlers**

Use username validation `^[A-Za-z0-9_-]{3,32}$`, password length `8..72`, session TTL seven days, `HttpOnly`, `SameSite=Lax`, `Path=/`, and JSON error bodies consistent with existing API errors.

- [ ] **Step 4: Run HTTP tests**

Run: `go test ./internal/httpapi`

Expected: PASS.

## Task 4: Frontend Auth UI, Proxy Cookies, and Leaderboard Page

**Files:**
- Modify: `frontend/app/api/[...path]/route.ts`
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/lib/api.ts`
- Modify: `frontend/components/app-shell.tsx`
- Add: `frontend/app/leaderboard/page.tsx`
- Modify or add frontend tests under `frontend/tests`

**Interfaces:**
- Consumes backend `/auth/*` and `/leaderboard`.
- Produces visible login/register/logout controls and leaderboard navigation.

- [ ] **Step 1: Write failing frontend tests**

Add tests proving:
- proxy forwards incoming `Cookie` to backend and backend `Set-Cookie` to browser;
- app shell can render logged-out and logged-in auth states;
- leaderboard page renders entries.

- [ ] **Step 2: Run frontend tests to verify RED**

Run: `cd frontend && npm test -- --run`

Expected: FAIL because new UI/API behavior is missing.

- [ ] **Step 3: Implement frontend changes**

Add API client functions, proxy cookie forwarding, app-shell auth form, logout button, leaderboard link, and leaderboard page. Keep UI compact in the existing top bar.

- [ ] **Step 4: Run frontend tests**

Run: `cd frontend && npm test -- --run`

Expected: PASS.

## Task 5: Documentation and Interview QA

**Files:**
- Modify: `README.md`
- Modify: `docs/openapi.yaml`
- Modify: `docs/codingjudge-interview-handbook.md`

**Interfaces:**
- Documents the exact API and the JWT vs Cookie Session design decision.

- [ ] **Step 1: Update docs**

Document register/login/logout/me, authenticated submission examples using `curl -c/-b`, and leaderboard response fields. Add interview QA explaining why this project uses server-side Cookie Session instead of JWT.

- [ ] **Step 2: Validate docs**

Run: `rg -n "JWT|Cookie Session|/auth/register|/leaderboard" README.md docs/openapi.yaml docs/codingjudge-interview-handbook.md`

Expected: all new concepts appear.

## Task 6: Full Verification

**Files:**
- All changed files.

- [ ] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run vet**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 3: Run frontend checks**

Run: `cd frontend && npm test -- --run && npm run lint && npm run typecheck`

Expected: PASS.

- [ ] **Step 4: Check git diff**

Run: `git diff --check`

Expected: no whitespace errors.

## Self-Review

- Spec coverage: login, required-auth submissions, user-bound submissions, leaderboard, and interview QA are covered.
- Scope control: contests, admin, OAuth, JWT refresh rotation, and email verification are intentionally excluded.
- Type consistency: store methods use `userID` strings and session token hashes; raw tokens are only used at HTTP cookie boundary.
