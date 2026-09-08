GOCACHE_DIR := $(CURDIR)/.cache/go-build

JUDGE_IMAGES := golang:1.25-alpine python:3.12-alpine gcc:13

.PHONY: test frontend-deps frontend-test frontend-build test-all build run-api run-worker upload-cases judge-images compose-up compose-down compose-config migrate migrate-reliable-workers migrate-hot20 fault-test smoke-minio-assets

test:
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) go test ./...

frontend-deps:
	rm -rf frontend/node_modules
	npm --prefix frontend ci

frontend-test: frontend-deps
	npm --prefix frontend run test:run

frontend-build: frontend-deps
	rm -rf frontend/.next
	npm --prefix frontend run build

test-all: test
	$(MAKE) frontend-test frontend-build

build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker
	go build -o bin/upload-cases ./cmd/upload-cases
	go build -o bin/migrate ./cmd/migrate

run-api:
	go run ./cmd/api

run-worker:
	DATABASE_URL='postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable' REDIS_ADDR=localhost:16379 JUDGE_WORKDIR=/tmp/codingjudge-sandbox go run ./cmd/worker

upload-cases:
	DATABASE_URL='postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable' MINIO_ENDPOINT=localhost:19000 MINIO_ACCESS_KEY=minioadmin MINIO_SECRET_KEY=minioadmin MINIO_BUCKET=codingjudge-assets go run ./cmd/upload-cases -cases-dir testdata/cases $${CASE_UPLOAD_FLAGS:--all}

judge-images:
	@for image in $(JUDGE_IMAGES); do \
		docker image inspect $$image >/dev/null 2>&1 || docker pull $$image; \
	done

compose-up: judge-images
	mkdir -p /tmp/codingjudge-sandbox
	docker compose up --build

compose-down:
	docker compose down

compose-config:
	docker compose config --quiet
	@! docker compose config | grep -E 'WORKER_URL|WORKER_ADDR'

migrate:
	DATABASE_URL="$${DATABASE_URL:-postgres://codingjudge:codingjudge@localhost:15432/codingjudge?sslmode=disable}" go run ./cmd/migrate -dir migrations

migrate-reliable-workers: migrate

migrate-hot20: migrate

fault-test:
	bash scripts/fault-test.sh

smoke-minio-assets:
	bash scripts/smoke-minio-assets.sh

.PHONY: observability-config observability-up load-smoke load-baseline load-worker-scale

observability-config:
	docker compose config --quiet
	@docker run --rm --entrypoint /bin/promtool -v $(CURDIR)/deploy/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro -v $(CURDIR)/deploy/prometheus/prometheus.rules.yml:/etc/prometheus/prometheus.rules.yml:ro prom/prometheus:v3.13.0 check config /etc/prometheus/prometheus.yml
	@docker run --rm --entrypoint /bin/promtool -v $(CURDIR)/deploy/prometheus/prometheus.rules.yml:/etc/prometheus/prometheus.rules.yml:ro prom/prometheus:v3.13.0 check rules /etc/prometheus/prometheus.rules.yml
	@docker compose --profile loadtest config --quiet

observability-up:
	docker compose up -d --build --scale worker=2

load-smoke:
	docker compose --profile loadtest run --rm k6 k6 run /scripts/problems.js --env CJ_VUS=1 --env CJ_DURATION=30s --summary-export=/results/smoke-problems-$$(date +%s).json

load-baseline:
	@set -eu; \
	cleanup() { docker compose up -d api >/dev/null; }; \
	trap cleanup EXIT INT TERM; \
	SUBMISSION_RATE_LIMIT_PER_MINUTE=100000 SUBMISSION_RATE_LIMIT_BURST=1000 docker compose up -d api; \
	docker compose --profile loadtest run --rm k6 k6 run /scripts/mixed.js --env CJ_VUS=20 --env CJ_DURATION=2m --env CJ_JUDGE_TIMEOUT_SECONDS=60 --summary-export=/results/baseline-$$(date +%s).json

load-worker-scale:
	bash scripts/run-worker-scale-benchmark.sh
