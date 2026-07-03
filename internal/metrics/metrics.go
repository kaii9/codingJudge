package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// App aggregates all codingjudge application metrics within a single registry.
type App struct {
	httpTotal        *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	submissionsTotal *prometheus.CounterVec
	outboxTotal      *prometheus.CounterVec
	outboxDuration   prometheus.Histogram
	queueOpsTotal    *prometheus.CounterVec
	queuePending     prometheus.Gauge
	workerSlots      prometheus.Gauge
	workerInFlight   prometheus.Gauge
	workerJobsTotal  *prometheus.CounterVec
	workerDuration   *prometheus.HistogramVec
	workerRetries    prometheus.Counter
	workerDeadLetter prometheus.Counter
	workerTakeovers  prometheus.Counter
	judgeCasesTotal  *prometheus.CounterVec
	judgeDuration    *prometheus.HistogramVec
}

// New creates a metrics.App and registers every collector into reg.
// It panics only when reg is nil or registration fails; callers must
// supply a valid prometheus.Registerer.
func New(reg prometheus.Registerer) *App {
	m := newApp()
	reg.MustRegister(
		m.httpTotal,
		m.httpDuration,
		m.submissionsTotal,
		m.outboxTotal,
		m.outboxDuration,
		m.queueOpsTotal,
		m.queuePending,
		m.workerSlots,
		m.workerInFlight,
		m.workerJobsTotal,
		m.workerDuration,
		m.workerRetries,
		m.workerDeadLetter,
		m.workerTakeovers,
		m.judgeCasesTotal,
		m.judgeDuration,
	)
	return m
}

func newApp() *App {
	return &App{
		httpTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codingjudge_http_requests_total",
			Help: "Total HTTP requests served.",
		}, []string{"method", "route", "status_class"}),

		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codingjudge_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),

		submissionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codingjudge_submissions_created_total",
			Help: "Total submissions accepted by the API.",
		}, []string{"language"}),

		outboxTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codingjudge_outbox_publish_total",
			Help: "Total outbox events published.",
		}, []string{"result"}),

		outboxDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "codingjudge_outbox_publish_duration_seconds",
			Help:    "Outbox publish operation latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}),

		queueOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codingjudge_queue_operations_total",
			Help: "Total Redis stream queue operations.",
		}, []string{"action", "result"}),

		queuePending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codingjudge_queue_pending_jobs",
			Help: "Number of pending jobs in the judge stream (XPENDING).",
		}),

		workerSlots: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codingjudge_worker_slots",
			Help: "Configured concurrency slots for this worker.",
		}),

		workerInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "codingjudge_worker_jobs_in_flight",
			Help: "Number of jobs currently being judged by this worker.",
		}),

		workerJobsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codingjudge_worker_jobs_total",
			Help: "Total jobs finished by worker.",
		}, []string{"result"}),

		workerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codingjudge_worker_job_duration_seconds",
			Help:    "Worker job end-to-end latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"language", "result"}),

		workerRetries: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "codingjudge_worker_retries_total",
			Help: "Total jobs that were retried.",
		}),

		workerDeadLetter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "codingjudge_worker_dead_letters_total",
			Help: "Total jobs sent to dead-letter stream.",
		}),

		workerTakeovers: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "codingjudge_worker_lease_takeovers_total",
			Help: "Total jobs claimed after a prior worker lease expiry.",
		}),

		judgeCasesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "codingjudge_judge_cases_total",
			Help: "Total test cases evaluated.",
		}, []string{"language", "result"}),

		judgeDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "codingjudge_judge_case_duration_seconds",
			Help:    "Test case evaluation latency in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"language"}),
	}
}

// ObserveHTTP records an HTTP request with the matched chi route pattern.
func (m *App) ObserveHTTP(method, route string, status int, duration time.Duration) {
	statusClass := statusClass(status)
	m.httpTotal.WithLabelValues(method, route, statusClass).Inc()
	m.httpDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}

// SubmissionCreated increments the submission creation counter for a language.
func (m *App) SubmissionCreated(language string) {
	m.submissionsTotal.WithLabelValues(language).Inc()
}

// ObserveOutboxPublish records a single outbox event publish attempt.
func (m *App) ObserveOutboxPublish(result string, duration time.Duration) {
	m.outboxTotal.WithLabelValues(result).Inc()
	m.outboxDuration.Observe(duration.Seconds())
}

// ObserveQueueOperation records a Redis stream operation outcome.
func (m *App) ObserveQueueOperation(action, result string) {
	m.queueOpsTotal.WithLabelValues(action, result).Inc()
}

// SetQueuePending sets the gauge for the number of pending Redis stream jobs.
func (m *App) SetQueuePending(value float64) {
	m.queuePending.Set(value)
}

// SetWorkerSlots sets the configured concurrency slots gauge.
func (m *App) SetWorkerSlots(value float64) {
	m.workerSlots.Set(value)
}

// WorkerJobStarted increments in-flight jobs gauge.
func (m *App) WorkerJobStarted() {
	m.workerInFlight.Inc()
}

// WorkerJobFinished decrements in-flight before recording duration and result.
func (m *App) WorkerJobFinished(language, result string, duration time.Duration) {
	m.workerInFlight.Dec()
	m.workerJobsTotal.WithLabelValues(result).Inc()
	m.workerDuration.WithLabelValues(language, result).Observe(duration.Seconds())
}

// WorkerRetry increments the retry counter.
func (m *App) WorkerRetry() {
	m.workerRetries.Inc()
}

// WorkerDeadLetter increments the dead-letter counter.
func (m *App) WorkerDeadLetter() {
	m.workerDeadLetter.Inc()
}

// WorkerLeaseTakeover increments the lease takeover counter.
func (m *App) WorkerLeaseTakeover() {
	m.workerTakeovers.Inc()
}

// ObserveJudgeCase records a single test case evaluation.
func (m *App) ObserveJudgeCase(language, result string, duration time.Duration) {
	m.judgeCasesTotal.WithLabelValues(language, result).Inc()
	m.judgeDuration.WithLabelValues(language).Observe(duration.Seconds())
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// Describe sends descriptors for all registered metrics.
func (m *App) Describe(ch chan<- *prometheus.Desc) {
	m.httpTotal.Describe(ch)
	m.httpDuration.Describe(ch)
	m.submissionsTotal.Describe(ch)
	m.outboxTotal.Describe(ch)
	m.outboxDuration.Describe(ch)
	m.queueOpsTotal.Describe(ch)
	m.queuePending.Describe(ch)
	m.workerSlots.Describe(ch)
	m.workerInFlight.Describe(ch)
	m.workerJobsTotal.Describe(ch)
	m.workerDuration.Describe(ch)
	m.workerRetries.Describe(ch)
	m.workerDeadLetter.Describe(ch)
	m.workerTakeovers.Describe(ch)
	m.judgeCasesTotal.Describe(ch)
	m.judgeDuration.Describe(ch)
}

// Collect gathers metric values from all registered metrics.
func (m *App) Collect(ch chan<- prometheus.Metric) {
	m.httpTotal.Collect(ch)
	m.httpDuration.Collect(ch)
	m.submissionsTotal.Collect(ch)
	m.outboxTotal.Collect(ch)
	m.outboxDuration.Collect(ch)
	m.queueOpsTotal.Collect(ch)
	m.queuePending.Collect(ch)
	m.workerSlots.Collect(ch)
	m.workerInFlight.Collect(ch)
	m.workerJobsTotal.Collect(ch)
	m.workerDuration.Collect(ch)
	m.workerRetries.Collect(ch)
	m.workerDeadLetter.Collect(ch)
	m.workerTakeovers.Collect(ch)
	m.judgeCasesTotal.Collect(ch)
	m.judgeDuration.Collect(ch)
}
