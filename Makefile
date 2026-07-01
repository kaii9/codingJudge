GOCACHE_DIR := $(CURDIR)/.cache/go-build

JUDGE_IMAGES := golang:1.25-alpine python:3.12-alpine gcc:13

.PHONY: test frontend-deps frontend-test frontend-build test-all build run-api run-worker judge-images compose-up compose-down

test:
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) go test ./...

frontend-deps:
	npm --prefix frontend ci

frontend-test: frontend-deps
	npm --prefix frontend run test:run

frontend-build: frontend-deps
	npm --prefix frontend run build

test-all: test frontend-test frontend-build

build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker

run-api:
	WORKER_URL=http://localhost:8081 go run ./cmd/api

run-worker:
	JUDGE_WORKDIR=/tmp/codingjudge-sandbox go run ./cmd/worker

judge-images:
	@for image in $(JUDGE_IMAGES); do \
		docker image inspect $$image >/dev/null 2>&1 || docker pull $$image; \
	done

compose-up: judge-images
	mkdir -p /tmp/codingjudge-sandbox
	docker compose up --build

compose-down:
	docker compose down
