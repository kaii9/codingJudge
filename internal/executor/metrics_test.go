package executor

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestExecutorMetricsTrackCapacityWaitAndResult(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	metrics := newPrometheusMetrics(registry, 3)
	metrics.executionStarted(25 * time.Millisecond)
	metrics.executionFinished("success")

	const expected = `
# HELP codingjudge_executor_slots Configured sandbox execution slots for this executor.
# TYPE codingjudge_executor_slots gauge
codingjudge_executor_slots 3
# HELP codingjudge_executor_executions_in_flight Number of sandbox batches currently executing.
# TYPE codingjudge_executor_executions_in_flight gauge
codingjudge_executor_executions_in_flight 0
# HELP codingjudge_executor_executions_total Total sandbox batches completed by result.
# TYPE codingjudge_executor_executions_total counter
codingjudge_executor_executions_total{result="success"} 1
`
	if err := testutil.GatherAndCompare(
		registry,
		strings.NewReader(expected),
		"codingjudge_executor_slots",
		"codingjudge_executor_executions_in_flight",
		"codingjudge_executor_executions_total",
	); err != nil {
		t.Fatal(err)
	}

	count := testutil.CollectAndCount(registry, "codingjudge_executor_queue_duration_seconds")
	if count != 1 {
		t.Fatalf("queue duration collectors = %d, want 1", count)
	}
}
