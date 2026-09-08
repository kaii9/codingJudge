package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaii9/codingJudge/internal/config"
	"github.com/kaii9/codingJudge/internal/httpapi"
	"github.com/kaii9/codingJudge/internal/metrics"
	"github.com/kaii9/codingJudge/internal/objectstore"
	"github.com/kaii9/codingJudge/internal/outbox"
	"github.com/kaii9/codingJudge/internal/problems"
	"github.com/kaii9/codingJudge/internal/queue"
	"github.com/kaii9/codingJudge/internal/ratelimit"
	"github.com/kaii9/codingJudge/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load(os.Getenv)
	if err := config.ValidateAPI(cfg); err != nil {
		slog.Error("invalid api configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, cleanupStore, err := buildStore(ctx, cfg)
	if err != nil {
		slog.Error("store setup failed", "error", err)
		os.Exit(1)
	}
	defer cleanupStore()

	// 创建独立的 Prometheus 注册器，注册 Go/Process 基础采集器和应用指标。
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsApp := metrics.New(registry)
	options := []httpapi.Option{
		httpapi.WithHTTPMetrics(metricsApp),
		httpapi.WithSubmissionMetrics(metricsApp),
		httpapi.WithSubmissionControlMetrics(metricsApp),
		httpapi.WithMetricsHandler(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})),
		httpapi.WithReadinessCheck("store", st.Ping),
		httpapi.WithSecureCookies(cfg.CookieSecure),
	}

	if cfg.QueueMode == config.QueueRedisStreams {
		client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		defer client.Close()
		submissionLimiter, err := ratelimit.NewRedisTokenBucket(
			client,
			cfg.SubmissionRateLimitPerMinute,
			cfg.SubmissionRateLimitBurst,
		)
		if err != nil {
			slog.Error("submission rate limiter setup failed", "error", err)
			os.Exit(1)
		}
		options = append(options, httpapi.WithSubmissionLimiter(submissionLimiter))
		relayID := processID("api")
		publisher := queue.NewRedisStreamsQueue(client, queue.DefaultJudgeStream, queue.DefaultJudgeGroup, relayID, queue.WithMetrics(metricsApp))
		relay := outbox.New(st, publisher, outbox.Config{RelayID: relayID, Metrics: metricsApp})
		go func() {
			if err := relay.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("outbox relay stopped", "error", err)
			}
		}()
		// 启动 Redis Pending 采样器，定期更新队列待处理数量。
		go metrics.SamplePending(ctx, metrics.RedisPendingFetcher{Client: client}, queue.DefaultJudgeStream, queue.DefaultJudgeGroup, metricsApp.SetQueuePending, 5*time.Second)
		options = append(options, httpapi.WithReadinessCheck("redis", func(ctx context.Context) error {
			return client.Ping(ctx).Err()
		}))
		slog.Info("outbox relay enabled", "relay_id", relayID,
			"submission_rate_per_minute", cfg.SubmissionRateLimitPerMinute,
			"submission_burst", cfg.SubmissionRateLimitBurst)
	} else {
		slog.Warn("memory mode has no cross-process judge relay")
	}

	if cfg.MinIOEndpoint != "" && cfg.MinIOAccessKey != "" && cfg.MinIOSecretKey != "" {
		objects, err := objectstore.NewMinIO(objectstore.MinIOConfig{
			Endpoint:  cfg.MinIOEndpoint,
			AccessKey: cfg.MinIOAccessKey,
			SecretKey: cfg.MinIOSecretKey,
			Bucket:    cfg.MinIOBucket,
			UseSSL:    cfg.MinIOUseSSL,
		})
		if err != nil {
			slog.Error("api object store setup failed", "error", err)
			os.Exit(1)
		}
		options = append(options,
			httpapi.WithObjectGetter(objects),
			httpapi.WithReadinessCheck("object_store", objects.Ping),
		)
		slog.Info("api object store enabled", "endpoint", cfg.MinIOEndpoint, "bucket", cfg.MinIOBucket)
	}

	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           httpapi.AccessLog(httpapi.NewServer(st, options...), slog.Default()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("api listening", "addr", cfg.APIAddr, "storage", cfg.StorageMode, "queue", cfg.QueueMode)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

type appStore interface {
	httpapi.ProblemStore
	store.OutboxStore
	Ping(context.Context) error
}

func buildStore(ctx context.Context, cfg config.Config) (appStore, func(), error) {
	if cfg.StorageMode == config.StoragePostgres {
		st, err := store.NewPostgresStore(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, func() {}, err
		}
		return st, st.Close, nil
	}
	return store.NewMemoryStore(problems.SampleProblems()), func() {}, nil
}

func processID(prefix string) string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%s-%d", prefix, hostname, os.Getpid())
}
