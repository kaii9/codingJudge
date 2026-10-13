package main

import (
	"context"
	"errors"
	"github.com/kaii9/codingJudge/internal/judgerpc"
	"github.com/kaii9/codingJudge/internal/metrics"
	"github.com/kaii9/codingJudge/internal/objectstore"
	"github.com/kaii9/codingJudge/internal/outbox"
	"github.com/kaii9/codingJudge/internal/platform"
	"github.com/kaii9/codingJudge/internal/queue"
	"github.com/kaii9/codingJudge/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("judge API stopped", "error", err)
		os.Exit(1)
	}
}
func run(ctx context.Context) error {
	token := os.Getenv("JUDGE_SERVICE_TOKEN")
	dsn := os.Getenv("DATABASE_URL")
	redisAddr := os.Getenv("REDIS_ADDR")
	if token == "" || dsn == "" || redisAddr == "" {
		return errors.New("judge service token, database and Redis required")
	}
	st, err := store.NewJudgePostgresStore(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer redisClient.Close()
	registry := prometheus.NewRegistry()
	app := metrics.New(registry)
	var objects judgerpc.ObjectGetter
	if os.Getenv("MINIO_ENDPOINT") != "" {
		objects, err = objectstore.NewMinIO(objectstore.MinIOConfig{Endpoint: os.Getenv("MINIO_ENDPOINT"), AccessKey: os.Getenv("MINIO_ACCESS_KEY"), SecretKey: os.Getenv("MINIO_SECRET_KEY"), Bucket: os.Getenv("MINIO_BUCKET"), UseSSL: os.Getenv("MINIO_USE_SSL") == "true"})
		if err != nil {
			return err
		}
	}
	publisher := queue.NewRedisStreamsQueue(redisClient, queue.DefaultJudgeStream, queue.DefaultJudgeGroup, "judge-relay", queue.WithMetrics(app))
	relay := outbox.New(st, publisher, outbox.Config{RelayID: "judge-api-" + os.Getenv("HOSTNAME"), Metrics: app})
	go func() { _ = relay.Run(ctx) }()
	completionPublisher := queue.NewRedisStreamsQueue(redisClient, platform.CompletionStream, platform.CompletionGroup, "completion-relay", queue.WithMetrics(app))
	completionRelay := outbox.New(store.CompletionOutbox{Store: st}, completionPublisher, outbox.Config{RelayID: "completion-" + os.Getenv("HOSTNAME"), Metrics: app})
	go func() { _ = completionRelay.Run(ctx) }()
	rpc := judgerpc.NewServer(st, objects, token)
	addr := os.Getenv("JUDGE_GRPC_ADDR")
	if addr == "" {
		addr = ":8092"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		check, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if st.Ping(check) != nil || redisClient.Ping(check).Err() != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	})
	metricsAddr := os.Getenv("JUDGE_METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9092"
	}
	httpListener, err := net.Listen("tcp", metricsAddr)
	if err != nil {
		return err
	}
	defer httpListener.Close()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 2)
	go func() { errs <- rpc.Serve(listener) }()
	go func() { errs <- server.Serve(httpListener) }()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errs:
	}
	grace, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { rpc.GracefulStop(); close(done) }()
	_ = server.Shutdown(grace)
	select {
	case <-done:
	case <-grace.Done():
		rpc.Stop()
		<-done
	}
	return serveErr
}
