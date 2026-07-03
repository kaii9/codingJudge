# Observability and Load Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Add application-level Prometheus metrics, a provisioned Grafana dashboard, and reproducible k6 benchmarks comparing one, two, and four judge workers.

**Architecture:** API and worker processes own custom Prometheus registries. Core packages depend on narrow recorder interfaces with nil/no-op behavior; the API serves /metrics on port 8080 and workers serve metrics on a configurable port 9091. Prometheus scrapes the API statically and discovers scaled workers through Docker DNS; Grafana and k6 remain optional local Compose services and never become judge-path dependencies.

**Tech Stack:** Go 1.25, prometheus/client_golang, chi, Redis Streams, PostgreSQL, Prometheus, Grafana, PromQL, k6, Docker Compose, Go testing.

---

## Scope and Invariants

- Local Docker Compose demonstration only. Authentication, Alertmanager, tracing, and long-term metrics storage are excluded.
- Every application metric uses the codingjudge_ prefix.
- Labels are fixed enums or bounded values. Submission IDs, problem IDs, source code, receipts, worker IDs, raw URL paths, and error strings are forbidden labels.
- Prometheus and Grafana outages must not stop API requests, outbox publication, queue consumption, or judging.
- HTTP route labels use chi templates such as /problems/{id}; unmatched requests use unmatched.
- Only the API samples global Redis Pending state, preventing duplicated gauges when workers scale.
- Benchmark reports include local machine conditions and are not production capacity guarantees.

## Target Metrics

| Metric | Type | Labels | Owner |
| --- | --- | --- | --- |
| codingjudge_http_requests_total | Counter | method, route, status_class | API |
| codingjudge_http_request_duration_seconds | Histogram | method, route | API |
| codingjudge_submissions_created_total | Counter | language | API |
| codingjudge_outbox_publish_total | Counter | result | API relay |
| codingjudge_outbox_publish_duration_seconds | Histogram | none | API relay |
| codingjudge_queue_operations_total | Counter | action, result | API/worker |
| codingjudge_queue_pending_jobs | Gauge | none | API sampler |
| codingjudge_worker_slots | Gauge | none | Worker |
| codingjudge_worker_jobs_in_flight | Gauge | none | Worker |
| codingjudge_worker_jobs_total | Counter | result | Worker |
| codingjudge_worker_job_duration_seconds | Histogram | language, result | Worker |
| codingjudge_worker_retries_total | Counter | none | Worker |
| codingjudge_worker_dead_letters_total | Counter | none | Worker |
| codingjudge_worker_lease_takeovers_total | Counter | none | Worker |
| codingjudge_judge_cases_total | Counter | language, result | Worker |
| codingjudge_judge_case_duration_seconds | Histogram | language | Worker |

Custom registries also register standard Go and process collectors.

### Task 1: Add the Metrics Registry

**Files:**
- Modify: go.mod
- Modify: go.sum
- Create: internal/metrics/metrics.go
- Create: internal/metrics/metrics_test.go

- [ ] **Step 1: Write failing registry tests**

Use prometheus.NewPedanticRegistry(), construct metrics.New(registry), record one observation through every public method, gather families, and assert all 16 target metric names exist. Inspect gathered label names and reject submission_id, problem_id, worker_id, receipt, path, and error.

- [ ] **Step 2: Verify RED**

~~~bash
go test ./internal/metrics -v
~~~

Expected: compilation fails because internal/metrics and metrics.New do not exist.

- [ ] **Step 3: Add the dependency**

~~~bash
go get github.com/prometheus/client_golang@v1.23.2
~~~

Commit the resolved compatible version; do not use a local replacement.

- [ ] **Step 4: Implement metrics.App**

Expose these methods:

~~~go
func New(reg prometheus.Registerer) *App
func (m *App) ObserveHTTP(method, route string, status int, duration time.Duration)
func (m *App) SubmissionCreated(language string)
func (m *App) ObserveOutboxPublish(result string, duration time.Duration)
func (m *App) ObserveQueueOperation(action, result string)
func (m *App) SetQueuePending(value float64)
func (m *App) SetWorkerSlots(value float64)
func (m *App) WorkerJobStarted()
func (m *App) WorkerJobFinished(language, result string, duration time.Duration)
func (m *App) WorkerRetry()
func (m *App) WorkerDeadLetter()
func (m *App) WorkerLeaseTakeover()
func (m *App) ObserveJudgeCase(language, result string, duration time.Duration)
~~~

Use prometheus.DefBuckets for HTTP and worker jobs. Use case buckets 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5 seconds. WorkerJobStarted increments in-flight; WorkerJobFinished always decrements before recording duration/result. Register only through the supplied registry, never global MustRegister.

- [ ] **Step 5: Verify and commit**

~~~bash
go test ./internal/metrics -v
go test ./...
git add go.mod go.sum internal/metrics
git commit -m "feat: add prometheus metrics registry"
~~~

### Task 2: Instrument HTTP and Submission Creation

**Files:**
- Modify: internal/httpapi/middleware.go
- Modify: internal/httpapi/middleware_test.go
- Modify: internal/httpapi/server.go
- Modify: internal/httpapi/server_test.go
- Modify: cmd/api/main.go

- [ ] **Step 1: Write failing tests**

Add a fake HTTP recorder. Route GET /submissions/{id} through chi, request /submissions/sub-123, and require route=/submissions/{id}, status=200, and positive duration. Require 404 to use unmatched and /metrics to be excluded. Add a server test proving only a successful 202 submission increments language=go.

- [ ] **Step 2: Verify RED**

~~~bash
go test ./internal/httpapi -run 'TestObserveHTTP|TestServerRecordsSubmission' -v
~~~

- [ ] **Step 3: Add narrow interfaces and server options**

~~~go
type HTTPMetrics interface {
    ObserveHTTP(method, route string, status int, duration time.Duration)
}
type SubmissionMetrics interface {
    SubmissionCreated(language string)
}
type Option func(*Server)
func WithMetricsHandler(http.Handler) Option
func WithSubmissionMetrics(SubmissionMetrics) Option
~~~

Change NewServer(store ProblemStore) to variadic NewServer(store ProblemStore, options ...Option), preserving existing callers. Register GET /metrics only when supplied. Record submission creation only after persistence succeeds.

- [ ] **Step 4: Add ObserveHTTP middleware**

After next.ServeHTTP, read chi.RouteContext(r.Context()).RoutePattern(). Map empty patterns to unmatched and status codes to 2xx/4xx/5xx in metrics.App. Skip /metrics to avoid self-scrape noise.

- [ ] **Step 5: Wire the API registry**

Create prometheus.NewRegistry(), register Go/process collectors, create one metrics.App, expose promhttp.HandlerFor, pass submission metrics into NewServer, and wrap the server with ObserveHTTP and existing AccessLog. Do not create multiple registries.

- [ ] **Step 6: Verify and commit**

~~~bash
go test ./internal/httpapi ./internal/metrics -v
go test ./...
git add cmd/api/main.go internal/httpapi
git commit -m "feat: expose api prometheus metrics"
~~~

### Task 3: Instrument Outbox and Redis Queue

**Files:**
- Modify: internal/outbox/relay.go
- Modify: internal/outbox/relay_test.go
- Modify: internal/queue/redis_streams.go
- Modify: internal/queue/redis_streams_test.go
- Modify: cmd/api/main.go

- [ ] **Step 1: Write failing tests**

Require outbox results success, error, and claim_lost. Require queue operations enqueue, dequeue, claim_pending, touch, ack, retry, dead_letter, and dead_letter_malformed with only success/error result labels. Verify malformed parse text never becomes a label.

- [ ] **Step 2: Add interfaces without breaking constructors**

Add Metrics to outbox.Config:

~~~go
type Metrics interface {
    ObserveOutboxPublish(result string, duration time.Duration)
}
~~~

Add variadic Redis options:

~~~go
type Metrics interface {
    ObserveQueueOperation(action, result string)
}
type RedisOption func(*RedisStreamsQueue)
func WithMetrics(Metrics) RedisOption
~~~

Make NewRedisStreamsQueue accept options ...RedisOption so all existing calls remain valid. Nil recorders perform no work.

- [ ] **Step 3: Instrument exact decision points**

Time each outbox event from before Enqueue through mark-published/mark-failed. Queue methods record once at their external operation boundary. Avoid double counting RetryJob inside moveAndAck.

- [ ] **Step 4: Wire API metrics and verify**

~~~bash
go test ./internal/outbox ./internal/queue -v
TEST_REDIS_ADDR=localhost:16379 go test -tags=integration ./internal/queue -v
go test ./...
git add cmd/api/main.go internal/outbox internal/queue
git commit -m "feat: instrument outbox and redis queue"
~~~

### Task 4: Instrument Worker Claims and Judge Cases

**Files:**
- Modify: internal/domain/domain.go
- Modify: internal/store/postgres_reliability.go
- Modify: internal/store/reliability.go
- Modify: internal/store/postgres_integration_test.go
- Modify: internal/store/reliability_test.go
- Modify: internal/judgeworker/processor.go
- Modify: internal/judgeworker/processor_test.go
- Modify: internal/judge/service.go
- Modify: internal/judge/judge_test.go

- [ ] **Step 1: Test accurate takeover classification**

Add LeaseTakeover bool to the expected SubmissionClaim contract. Test first claim=false, explicit infrastructure retry=false, and claim after an expired running lease=true in both PostgreSQL and memory stores.

- [ ] **Step 2: Implement classification**

Set LeaseTakeover only when pre-update status is running, a prior lease expiry exists, and now is not before that expiry. Do not infer takeover from Attempts because retries also increment attempts.

- [ ] **Step 3: Test Processor metrics**

Add WorkerMetrics to judgeworker.Config. Tests must cover accepted, retry, dead-letter, lease-lost, and expired-lease takeover. Increment in-flight only after ClaimAcquired; every started metric must have exactly one finish.

Allowed result labels:

~~~text
accepted
wrong_answer
runtime_error
time_limit_exceeded
internal_error
retry
dead_letter
lease_lost
infrastructure_error
~~~

- [ ] **Step 4: Test and implement judge case metrics**

Add variadic options to NewService:

~~~go
type Metrics interface {
    ObserveJudgeCase(language, result string, duration time.Duration)
}
func WithMetrics(Metrics) Option
~~~

A two-case accepted run emits two passed observations. Wrong answer, timeout, and runtime error record only cases processed through the terminal case. Infrastructure errors do not fabricate outcomes. Convert RunResult.Duration milliseconds to seconds.

- [ ] **Step 5: Verify and commit**

~~~bash
go test ./internal/domain ./internal/store ./internal/judge ./internal/judgeworker -v
TEST_DATABASE_URL='postgres://codingjudge:codingjudge@localhost:15432/codingjudge_test?sslmode=disable' go test -tags=integration ./internal/store -v
go test -race ./internal/judge ./internal/judgeworker ./internal/store
git add internal/domain internal/store internal/judge internal/judgeworker
git commit -m "feat: instrument judge worker lifecycle"
~~~

### Task 5: Serve Worker Metrics and Sample Redis Pending

**Files:**
- Modify: internal/config/config.go
- Modify: internal/config/config_test.go
- Create: internal/metrics/server.go
- Create: internal/metrics/server_test.go
- Create: internal/metrics/redis_sampler.go
- Create: internal/metrics/redis_sampler_test.go
- Modify: cmd/api/main.go
- Modify: cmd/worker/main.go

- [ ] **Step 1: Test worker metrics configuration**

WorkerConfig.MetricsAddr defaults to :9091, accepts WORKER_METRICS_ADDR=:9191, and disables serving with WORKER_METRICS_ADDR=off. Reject malformed values.

- [ ] **Step 2: Test metrics server lifecycle**

With an ephemeral listener, verify /metrics responds, context cancellation shuts down cleanly, and bind failure is returned before worker processing starts.

- [ ] **Step 3: Test Redis Pending sampler**

Wrap XPending behind a minimal interface. Successful samples set the gauge; Redis errors log and continue; cancellation exits promptly. Default interval is five seconds and tests inject a short interval.

- [ ] **Step 4: Wire worker registry**

Create one custom registry per worker process, register Go/process collectors, set worker slots to configured concurrency, and pass the recorder to queue, processor, and judge. Start metrics serving before Pool.Run. A local bind failure stops startup; later Prometheus scrape failures do not affect the worker.

- [ ] **Step 5: Start the sampler only in API**

Reuse the API Redis client and queue constants. Run sampler beside the relay. Sampling errors never cancel the API.

- [ ] **Step 6: Verify and commit**

~~~bash
go test ./internal/config ./internal/metrics -v
go test ./...
go vet ./...
git add cmd/api/main.go cmd/worker/main.go internal/config internal/metrics
git commit -m "feat: serve worker and queue metrics"
~~~

### Task 6: Provision Prometheus

**Files:**
- Create: deploy/prometheus/prometheus.yml
- Create: deploy/prometheus/prometheus.rules.yml
- Modify: docker-compose.yml
- Modify: Makefile

- [ ] **Step 1: Add scrape configuration**

Use a five-second scrape interval:

~~~yaml
scrape_configs:
  - job_name: codingjudge-api
    static_configs:
      - targets: [api:8080]
  - job_name: codingjudge-worker
    dns_sd_configs:
      - names: [worker]
        type: A
        port: 9091
        refresh_interval: 5s
~~~

Add recording rules for HTTP rate, 5xx ratio, HTTP P95, worker throughput, and worker-job P95. Aggregate histogram buckets by le before histogram_quantile.

- [ ] **Step 2: Add Compose service**

Pin Prometheus to prom/prometheus:v3.13.0, expose host port 9090, mount configuration read-only, persist /prometheus, and add a health check. Worker exposes 9091 internally and sets WORKER_METRICS_ADDR=:9091. Workers do not depend on Prometheus.

- [ ] **Step 3: Add Make validation**

Add observability-config using promtool check config and promtool check rules, plus observability-up scaling two workers.

- [ ] **Step 4: Verify discovery and non-critical behavior**

~~~bash
make observability-config
docker compose up -d --build --scale worker=4
docker compose exec -T prometheus promtool query instant http://localhost:9090 'count(up{job="codingjudge-worker"} == 1)'
~~~

Expected worker count: 4. Stop Prometheus, submit accepted code, and prove judging still reaches accepted.

- [ ] **Step 5: Commit**

~~~bash
git add deploy/prometheus docker-compose.yml Makefile
git commit -m "feat: add prometheus compose monitoring"
~~~

### Task 7: Provision Grafana

**Files:**
- Create: deploy/grafana/provisioning/datasources/prometheus.yml
- Create: deploy/grafana/provisioning/dashboards/dashboards.yml
- Create: deploy/grafana/dashboards/gojudge-overview.json
- Create: scripts/verify-grafana.sh
- Modify: docker-compose.yml
- Modify: Makefile
- Modify: .env.example

- [ ] **Step 1: Provision data source and service**

Use stable data-source UID prometheus, URL http://prometheus:9090, proxy access, and default=true. Pin Grafana to grafana/grafana:13.0.2, expose port 3001, mount provisioning/dashboard files read-only, persist /var/lib/grafana, disable analytics, and document local demo credentials.

- [ ] **Step 2: Build dashboard UID gojudge-overview**

Create rows API, Queue / Outbox, Worker, and Judge. Required PromQL panels:

~~~text
sum(rate(codingjudge_http_requests_total[1m]))
sum(rate(codingjudge_http_requests_total{status_class="5xx"}[5m])) / clamp_min(sum(rate(codingjudge_http_requests_total[5m])), 1e-9)
histogram_quantile(0.50, sum by (le) (rate(codingjudge_http_request_duration_seconds_bucket[5m])))
histogram_quantile(0.95, sum by (le) (rate(codingjudge_http_request_duration_seconds_bucket[5m])))
histogram_quantile(0.99, sum by (le) (rate(codingjudge_http_request_duration_seconds_bucket[5m])))
sum by (language) (rate(codingjudge_submissions_created_total[5m]))
codingjudge_queue_pending_jobs
sum by (result) (rate(codingjudge_outbox_publish_total[5m]))
sum(rate(codingjudge_worker_retries_total[5m])) or vector(0)
sum(rate(codingjudge_worker_dead_letters_total[5m])) or vector(0)
sum(codingjudge_worker_slots)
sum(codingjudge_worker_jobs_in_flight)
sum by (result) (rate(codingjudge_worker_jobs_total[5m]))
histogram_quantile(0.95, sum by (le, language) (rate(codingjudge_worker_job_duration_seconds_bucket[5m])))
sum(increase(codingjudge_worker_lease_takeovers_total[15m]))
sum by (language, result) (rate(codingjudge_judge_cases_total[5m]))
sum(rate(codingjudge_worker_jobs_total{result="accepted"}[5m])) / clamp_min(sum(rate(codingjudge_worker_jobs_total[5m])), 1e-9)
histogram_quantile(0.95, sum by (le, language) (rate(codingjudge_judge_case_duration_seconds_bucket[5m])))
~~~

Use seconds for latency, percent for ratios, and short units for counts/rates.

- [ ] **Step 3: Add API verification script**

Wait for /api/health, authenticate with local credentials, assert data-source UID prometheus exists, fetch dashboard UID gojudge-overview, and verify all four row titles.

- [ ] **Step 4: Verify and commit**

~~~bash
docker compose up -d prometheus grafana
bash scripts/verify-grafana.sh
curl -fsS http://localhost:3001/api/health
git add deploy/grafana scripts/verify-grafana.sh docker-compose.yml Makefile .env.example
git commit -m "feat: provision gojudge grafana dashboard"
~~~

### Task 8: Add k6 Workloads

**Files:**
- Create: loadtest/lib/client.js
- Create: loadtest/lib/programs.js
- Create: loadtest/problems.js
- Create: loadtest/submissions.js
- Create: loadtest/mixed.js
- Create: loadtest/cleanup.sql
- Create: loadtest/results/.gitkeep
- Modify: docker-compose.yml
- Modify: .gitignore
- Modify: Makefile

- [ ] **Step 1: Implement shared helpers**

BASE_URL defaults to http://api:8080. Helpers cover list/detail/create/poll with k6 checks and a bounded logical-failure Rate. Polling stops on terminal status and fails after JUDGE_TIMEOUT_SECONDS=30. Accepted Go/C++/Python programs target sum and contain the comment k6-loadtest.

- [ ] **Step 2: Implement problems.js**

Alternate GET /problems and GET /problems/target-pair. Thresholds:

~~~javascript
{
  http_req_failed: ["rate<0.01"],
  http_req_duration: ["p(95)<300"],
  checks: ["rate>0.99"],
}
~~~

Smoke defaults to one VU for 30 seconds; environment variables override VUs and duration.

- [ ] **Step 3: Implement submissions.js and mixed.js**

submissions.js uses constant-arrival-rate, tracks enqueue HTTP P95 separately from custom judge_terminal_duration, and polls every submission. Enqueue P95 must be below 500ms and logical failures below 1%. mixed.js uses deterministic 80% reads and 20% submissions. Baselines must not intentionally create timeouts, runtime errors, or dead letters.

- [ ] **Step 4: Add k6 Compose profile and Make targets**

Pin k6 to grafana/k6:2.0.0, place it under profile loadtest, mount scripts read-only and results writable. Ordinary compose-up must not start k6. Add load-smoke (1 VU, 30s) and load-baseline (20 VUs, 2m) targets with JSON summary export. Ignore generated result JSON.

- [ ] **Step 5: Add safe cleanup**

cleanup.sql deletes only submissions whose code contains k6-loadtest; outbox rows cascade. Cleanup first asserts Redis Pending is zero. It must not flush Redis, trim streams, or delete ordinary submissions.

- [ ] **Step 6: Verify and commit**

~~~bash
make load-smoke
docker compose --profile loadtest config --quiet
make test
git add loadtest docker-compose.yml Makefile .gitignore
git commit -m "test: add reproducible k6 workloads"
~~~

### Task 9: Automate Worker Scaling and Publish Results

**Files:**
- Create: scripts/run-worker-scale-benchmark.sh
- Create: scripts/render-benchmark-report.go
- Create: scripts/render_benchmark_report_test.go
- Create: scripts/testdata/k6-worker-1.json
- Create: scripts/testdata/k6-worker-2.json
- Create: scripts/testdata/k6-worker-4.json
- Create after running benchmark: docs/benchmarks/2026-07-03-worker-scaling.md
- Modify: README.md
- Modify: docs/development-plan.md
- Modify: .github/workflows/ci.yml
- Modify: Makefile

- [ ] **Step 1: Test report rendering**

Fixtures must produce an ordered table:

~~~text
Workers | Submission rate | HTTP P95 | Judge P95 | Failure rate | Peak pending
~~~

Missing metrics return non-zero. Values use consistent units and rounding.

- [ ] **Step 2: Implement report renderer**

Read three k6 summaries plus machine metadata and emit date, Git commit, OS/architecture, logical CPUs, memory, Docker version, scenario settings, result table, evidence-based interpretation, and local-test limitations.

- [ ] **Step 3: Implement benchmark orchestration**

The script records machine metadata, warms three judge images, loops worker counts 1/2/4, waits for Prometheus to report exactly N healthy targets, runs the same fixed-rate submission scenario, captures summary and peak Pending, waits for Pending=0, and renders the report. A trap restores two workers on success, failure, or interruption. Threshold failure stops the run.

- [ ] **Step 4: Run and validate**

~~~bash
make load-worker-scale
~~~

Reject and repeat runs affected by image downloads, missing targets, non-zero final Pending, application errors, or CPU throttling. Never hand-edit measured values.

- [ ] **Step 5: Update documentation and CI**

README links Prometheus :9090, Grafana :3001, benchmark commands, and the generated report. Mark observability/load testing complete only after the report exists. CI runs Go tests, focused race tests, promtool config/rule checks, and docker compose --profile loadtest config --quiet. Do not run full sandbox benchmarks on shared GitHub runners.

- [ ] **Step 6: Final verification**

~~~bash
make test
go test -race ./...
go vet ./...
make observability-config
docker compose --profile loadtest config --quiet
docker compose up -d --build --scale worker=2
bash scripts/verify-grafana.sh
make load-smoke
COMPOSE_PROJECT_NAME=codingjudge make fault-test
docker compose exec -T redis redis-cli XPENDING judge:submissions judge-workers
git diff --check
~~~

Expected: all checks pass; API and every worker target are up; Grafana is provisioned; smoke thresholds pass; fault recovery succeeds; Pending is 0; stopping Prometheus/Grafana does not affect judging.

- [ ] **Step 7: Commit**

~~~bash
git add scripts docs/benchmarks README.md docs/development-plan.md .github/workflows/ci.yml Makefile
git commit -m "docs: publish observability benchmark results"
~~~

## Completion Criteria

1. API and every scaled worker expose metrics without high-cardinality labels.
2. Prometheus discovers one, two, and four workers without configuration edits.
3. Grafana starts with a usable four-row dashboard and no manual setup.
4. API and judging remain functional while Prometheus and Grafana are stopped.
5. k6 smoke and baseline thresholds pass.
6. A reproducible 1/2/4-worker report contains machine metadata and measured values.
7. Redis Pending is zero after benchmarks and fault recovery.
8. Unit, integration, race, vet, Prometheus, Compose, Grafana, and k6 verification commands pass.
