# MinIO Hot20 Compile Error CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add an independent `compile_error` judge status, migrate Hot20 case assets into the MinIO upload path, and make CI verify the MinIO object-storage path.

**Architecture:** Keep PostgreSQL as the source of truth for problems, test-case metadata, submissions, leases, and artifact metadata. Use MinIO only for object-backed case files and judge artifacts. Let DockerRunner identify compile-stage failures; Service maps compile-stage failure to `compile_error`, while runtime failures remain `runtime_error`.

**Tech Stack:** Go 1.25, PostgreSQL, Redis Streams, MinIO, Docker Compose, GitHub Actions, Next.js.

## Global Constraints

- Do not execute untrusted code in the API service; only judge worker may run Docker.
- Keep API + Worker separated.
- Keep MinIO object keys internal; API may serve artifact content but must not expose raw object keys.
- Use tests before implementation for behavior changes.
- Keep local teaching documents untracked unless explicitly requested.

---

### Task 1: Independent Compile Error Status

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `internal/judge/service.go`
- Modify: `internal/judge/docker_runner.go`
- Modify: `internal/judge/judge_test.go`
- Modify: `internal/judge/docker_runner_test.go`
- Modify: `internal/judgeworker/processor.go`
- Modify: `internal/judgeworker/processor_test.go`
- Modify: `docs/openapi.yaml`
- Modify: `README.md`

**Interfaces:**
- Produces: `domain.StatusCompileError SubmissionStatus = "compile_error"`
- Produces: `judge.RunStage`, with compile results marked as `StageCompile`
- Consumes: existing `domain.JudgeResult`, `judge.RunResult`, worker metrics labels

- [x] Add failing tests proving compile-stage non-zero and timeout map to `compile_error`.
- [x] Add failing tests proving runtime non-zero remains `runtime_error`.
- [x] Add failing tests proving `compile_error` is terminal and appears in worker metric labels.
- [x] Implement `StatusCompileError` and stage-aware mapping.
- [x] Update OpenAPI and README status docs.
- [x] Run `go test ./internal/domain ./internal/judge ./internal/judgeworker ./internal/httpapi`.

### Task 2: Hot20 MinIO Case Assets

**Files:**
- Modify: `internal/caseassets/uploader.go`
- Modify: `internal/caseassets/uploader_test.go`
- Modify: `cmd/upload-cases/main.go`
- Create: `testdata/cases/<problem-id>/*.in`
- Create: `testdata/cases/<problem-id>/*.out`
- Modify: `Makefile`
- Modify: `docker-compose.yml`
- Modify: `README.md`

**Interfaces:**
- Produces: `caseassets.DiscoverProblems(root string) ([]string, error)`
- Produces: CLI flag `-all`, uploading every problem directory under `testdata/cases`
- Consumes: existing `Uploader.UploadProblem(ctx, root, problemID)`

- [x] Add failing tests for deterministic problem discovery and `-all` behavior helper.
- [x] Add Hot20 case files derived from `migrations/004_hot20_problem_set.sql`.
- [x] Implement discovery and CLI `-all`.
- [x] Update `Makefile` and Compose uploader defaults to support all cases.
- [x] Run `go test ./internal/caseassets ./cmd/upload-cases` and `go test ./...`.

### Task 3: CI MinIO Coverage

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`

**Interfaces:**
- Consumes: existing `scripts/smoke-minio-assets.sh`
- Consumes: integration tests in `internal/objectstore` and `internal/store`

- [x] Add MinIO service to the Go CI job.
- [x] Add `TEST_MINIO_*` env vars.
- [x] Extend integration test command to include `./internal/objectstore`.
- [x] Add Docker Compose MinIO smoke job step.
- [x] Run local YAML/Compose checks and final test suite.

### Task 4: Final Verification

**Files:**
- No new files expected.

- [x] Run `go test ./...`.
- [x] Run `go vet ./...`.
- [x] Run `go build ./cmd/...`.
- [x] Run `docker compose config >/tmp/codingjudge-compose.yml`.
- [x] Run `bash scripts/smoke-minio-assets.sh`.
- [x] Run integration tests with PostgreSQL and MinIO.
- [x] Run frontend verification with Node 25 path if needed.
- [x] Commit tracked project changes, leaving local teaching docs untracked.
