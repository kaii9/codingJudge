package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestAllTargetMetricsRegistered(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	app := New(registry)

	// Record one observation for every method.
	app.ObserveHTTP("GET", "/problems/{id}", 200, 5*time.Millisecond)
	app.ObserveHTTP("GET", "/submissions", 404, 1*time.Millisecond)
	app.SubmissionCreated("go")
	app.ObserveOutboxPublish("success", 10*time.Millisecond)
	app.ObserveQueueOperation("enqueue", "success")
	app.SetQueuePending(3)
	app.SetWorkerSlots(4)
	app.WorkerJobStarted()
	app.WorkerJobFinished("go", "accepted", 100*time.Millisecond)
	app.WorkerRetry()
	app.WorkerDeadLetter()
	app.WorkerLeaseTakeover()
	app.ObserveJudgeCase("go", "accepted", 15*time.Millisecond)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	metricNames := make(map[string]bool)
	for _, f := range families {
		name := f.GetName()
		if !strings.HasPrefix(name, "codingjudge_") {
			t.Errorf("metric %q should use codingjudge_ prefix", name)
		}
		metricNames[name] = true
	}

	required := []string{
		"codingjudge_http_requests_total",
		"codingjudge_http_request_duration_seconds",
		"codingjudge_submissions_created_total",
		"codingjudge_outbox_publish_total",
		"codingjudge_outbox_publish_duration_seconds",
		"codingjudge_queue_operations_total",
		"codingjudge_queue_pending_jobs",
		"codingjudge_worker_slots",
		"codingjudge_worker_jobs_in_flight",
		"codingjudge_worker_jobs_total",
		"codingjudge_worker_job_duration_seconds",
		"codingjudge_worker_retries_total",
		"codingjudge_worker_dead_letters_total",
		"codingjudge_worker_lease_takeovers_total",
		"codingjudge_judge_cases_total",
		"codingjudge_judge_case_duration_seconds",
	}

	for _, name := range required {
		if !metricNames[name] {
			t.Errorf("missing metric: %s", name)
		}
	}

	// Verify forbidden labels are not present.
	forbiddenLabels := map[string]bool{
		"submission_id": true,
		"problem_id":    true,
		"worker_id":     true,
		"receipt":       true,
		"path":          true,
		"source_code":   true,
		"code":          true,
		"error":         true,
	}

	for _, f := range families {
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if forbiddenLabels[lp.GetName()] {
					t.Errorf("metric %q uses forbidden label %q", f.GetName(), lp.GetName())
				}
			}
		}
	}
}

func TestHTTPMetricsCollector(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	app := New(registry)

	app.ObserveHTTP("POST", "/submissions", 202, 42*time.Millisecond)

	// Verify status_class label is normalized to 2xx.
	problem, err := testutil.CollectAndLint(registry, "codingjudge_http_requests_total")
	if err != nil {
		t.Fatal(err)
	}
	if problem != nil {
		t.Errorf("lint: %v", problem)
	}

	const metadata = `
# HELP codingjudge_http_requests_total Total HTTP requests served.
# TYPE codingjudge_http_requests_total counter
`
	expected := `
codingjudge_http_requests_total{method="POST",route="/submissions",status_class="2xx"} 1
`
	if err := testutil.CollectAndCompare(registry, strings.NewReader(metadata+expected), "codingjudge_http_requests_total"); err != nil {
		t.Error(err)
	}
}

func TestWorkerJobInFlightTracking(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	app := New(registry)

	app.WorkerJobStarted()
	app.WorkerJobStarted()

	// Verify in-flight is 2.
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() == "codingjudge_worker_jobs_in_flight" {
			if len(f.GetMetric()) != 1 {
				t.Fatalf("expected 1 metric, got %d", len(f.GetMetric()))
			}
			if val := f.GetMetric()[0].GetGauge().GetValue(); val != 2 {
				t.Errorf("expected in-flight=2, got %v", val)
			}
		}
	}

	app.WorkerJobFinished("go", "accepted", 100*time.Millisecond)

	families, err = registry.Gather()
	if err != nil {
		t.Fatalf("gather after finish: %v", err)
	}
	for _, f := range families {
		if f.GetName() == "codingjudge_worker_jobs_in_flight" {
			if val := f.GetMetric()[0].GetGauge().GetValue(); val != 1 {
				t.Errorf("expected in-flight=1 after finish, got %v", val)
			}
		}
	}
}
