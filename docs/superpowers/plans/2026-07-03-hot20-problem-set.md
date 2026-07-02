# Hot 20 Problem Set Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 20 original, fully judgeable interview-style problems with normalized metadata, validated hidden cases, and frontend collection/search/difficulty filtering.

**Architecture:** PostgreSQL migration `004` extends problem metadata, normalizes tags, and seeds deterministic cases. Existing list/detail APIs expose metadata but continue stripping hidden tests; the Next.js rail filters the complete 22-item list client-side. Integration tests apply every migration in sorted order and validate seeded outputs with trusted Go reference solvers.

**Tech Stack:** Go 1.25, PostgreSQL 16/pgx, SQL migrations, Next.js 16, React, TypeScript, Vitest, Playwright, Docker Compose.

---

### Task 1: Add Problem Metadata Contracts

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `internal/domain/domain_test.go`
- Modify: `frontend/lib/types.ts`
- Create: `migrations/004_hot20_problem_set.sql`

- [ ] Write failing Go tests for difficulty/collection JSON values and metadata fields.
- [ ] Run `go test ./internal/domain -v`; expect compile failure for missing types.
- [ ] Add `ProblemDifficulty` (`easy`, `medium`, `hard`), `ProblemCollection` (`starter`, `hot20`), and `Difficulty`, `Collection`, `SortOrder`, `Tags` fields to `domain.Problem`.
- [ ] Extend the TypeScript `Problem` interface with matching unions and fields.
- [ ] Create migration DDL for metadata columns, check constraints through idempotent `DO` blocks, `problem_tags`, and collection/tag indexes.
- [ ] Run `go test ./internal/domain && git diff --check`; expect PASS.
- [ ] Commit with `feat: add problem library metadata`.

### Task 2: Seed the Hot 20 Catalog and Cases

**Files:**
- Modify: `migrations/004_hot20_problem_set.sql`
- Create: `docs/hot20-input-formats.md`

- [ ] Add the exact 20 stable IDs, original descriptions, difficulties, collection order, and two-to-four tags from the approved spec.
- [ ] Add at least six deterministic test cases per problem, including empty/minimum, duplicate, no-solution, tie-breaking, and 64-bit cases where applicable.
- [ ] Make reruns idempotent by upserting problem rows, replacing managed tags, and deleting/reinserting only Hot 20 test cases.
- [ ] Document every stdin/stdout grammar and deterministic multi-answer ordering in `docs/hot20-input-formats.md`.
- [ ] Apply migration twice against the integration database and query counts; expect 20 Hot problems, 2 Starter problems, no duplicate tags, and at least 120 Hot test cases.
- [ ] Commit with `feat: seed hot20 problem catalog`.

### Task 3: Validate Every Seeded Expected Output

**Files:**
- Modify: `internal/store/postgres_integration_test.go`
- Create: `internal/problems/hot20_reference_test.go`

- [ ] Change integration migration setup to discover and sort `migrations/*.sql` instead of listing three filenames.
- [ ] Write a failing integration test that queries all Hot cases and dispatches by problem ID; initially fail on an unknown solver.
- [ ] Implement trusted reference solvers for all 20 grammars, including canonical sorting for triples/combinations and lexicographically smallest topological order.
- [ ] Assert exactly 20 IDs, two-to-four tags each, at least six cases each, and normalized reference output equality for every case.
- [ ] Reapply migration and rerun validation to prove idempotency.
- [ ] Run `TEST_DATABASE_URL=postgres://codingjudge:codingjudge@localhost:15432/codingjudge_test?sslmode=disable go test -tags=integration ./internal/problems ./internal/store -v`; expect PASS.
- [ ] Commit with `test: validate hot20 seed outputs`.

### Task 4: Return Metadata Through Memory, PostgreSQL, and HTTP

**Files:**
- Modify: `internal/problems/sample.go`
- Modify: `internal/store/postgres.go`
- Modify: `internal/store/memory_test.go`
- Modify: `internal/store/postgres_integration_test.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `docs/openapi.yaml`

- [ ] Write failing store tests requiring Hot-before-Starter ordering, alphabetically sorted tags, and metadata on detail records.
- [ ] Write failing HTTP tests requiring metadata while continuing to reject `testCases` leakage.
- [ ] Update sample problems with Starter metadata.
- [ ] Update PostgreSQL list/detail queries to load normalized tags in deterministic order and order by collection, `sort_order`, and ID.
- [ ] Update OpenAPI `Problem` schema and examples.
- [ ] Run `go test ./internal/store ./internal/httpapi && go test -tags=integration ./internal/store`; expect PASS.
- [ ] Commit with `feat: expose problem library metadata`.

### Task 5: Build Collection Search and Difficulty Filtering

**Files:**
- Modify: `frontend/components/problem-rail.tsx`
- Modify: `frontend/components/problem-statement.tsx`
- Modify: `frontend/app/globals.css`
- Modify: `frontend/tests/problem-views.test.tsx`
- Modify: `frontend/tests/app-shell.test.tsx`
- Modify: `frontend/e2e/responsive.spec.ts`

- [ ] Write failing Vitest cases for default Hot 20, active Starter selection, case-insensitive title/tag search, combined collection+difficulty filters, and empty state.
- [ ] Convert `ProblemRail` to a client component with collection segmented controls, search input, and difficulty select.
- [ ] Render stable difficulty labels and tags in list rows and the statement header.
- [ ] Add responsive styles with constrained controls, internal list scrolling, and no horizontal overflow.
- [ ] Update test fixtures with full metadata.
- [ ] Run `npm --prefix frontend run test:run`; expect PASS.
- [ ] Run responsive Playwright checks against Compose; expect desktop/mobile PASS with intentional cross-viewport skips only.
- [ ] Commit with `feat: add hot20 problem filtering`.

### Task 6: Add Migration Operations and Documentation

**Files:**
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `docs/development-plan.md`

- [ ] Add `migrate-hot20` executing `/docker-entrypoint-initdb.d/004_hot20_problem_set.sql` with `ON_ERROR_STOP=1`.
- [ ] Document the 20+2 collection model, topic coverage, filter controls, migration command, and reference-validation command.
- [ ] Mark the curated library phase complete while retaining observability/load testing as the next backend priority.
- [ ] Run `make migrate-hot20` twice against the existing volume and verify stable counts.
- [ ] Commit with `docs: document hot20 problem library`.

### Task 7: End-to-End Verification and Review

**Files:**
- Modify only files required by verified defects.

- [ ] Run `make test`, `go test -race ./...`, and `go vet ./...`.
- [ ] Run PostgreSQL/Redis integration tests including all reference solvers.
- [ ] Run frontend lint, typecheck,  unit tests, and production build.
- [ ] Rebuild Compose with two workers and apply `make migrate-hot20`.
- [ ] Use Go, C++, and Python across representative hash/tree/graph/DP submissions and assert terminal results.
- [ ] Run Playwright judge/responsive tests and the existing worker fault test.
- [ ] Assert `XPENDING judge:submissions judge-workers` is zero.
- [ ] Review for hidden-test leakage, migration duplication, ambiguous output formats, filter overflow, and unrelated changes.
- [ ] Run `git diff --check` and confirm a clean worktree.
