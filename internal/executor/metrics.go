package executor

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type executionMetrics interface {
	executionStarted(time.Duration)
	executionFinished(string)
}

type prometheusMetrics struct {
	inFlight      prometheus.Gauge
	queueDuration prometheus.Histogram
	executions    *prometheus.CounterVec
}

func WithPrometheusMetrics(reg prometheus.Registerer, slots int) ServerOption {
	metrics := newPrometheusMetrics(reg, slots)
	return func(server *Server) {
		server.executionMetrics = metrics
	}
}

func newPrometheusMetrics(reg prometheus.Registerer, slots int) executionMetrics {
	capacity := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codingjudge_executor_slots",
		Help: "Configured sandbox execution slots for this executor.",
	})
	inFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codingjudge_executor_executions_in_flight",
		Help: "Number of sandbox batches currently executing.",
	})
	queueDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "codingjudge_executor_queue_duration_seconds",
		Help:    "Time an authenticated sandbox request waits for an executor slot.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
	executions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codingjudge_executor_executions_total",
		Help: "Total sandbox batches completed by result.",
	}, []string{"result"})
	reg.MustRegister(capacity, inFlight, queueDuration, executions)
	capacity.Set(float64(slots))
	return &prometheusMetrics{inFlight: inFlight, queueDuration: queueDuration, executions: executions}
}

func (m *prometheusMetrics) executionStarted(queueDuration time.Duration) {
	m.queueDuration.Observe(queueDuration.Seconds())
	m.inFlight.Inc()
}

func (m *prometheusMetrics) executionFinished(result string) {
	m.inFlight.Dec()
	m.executions.WithLabelValues(result).Inc()
}
