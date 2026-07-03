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

	"github.com/kai/codingjudge/internal/config"
	"github.com/kai/codingjudge/internal/httpapi"
	"github.com/kai/codingjudge/internal/metrics"
	"github.com/kai/codingjudge/internal/outbox"
	"github.com/kai/codingjudge/internal/problems"
	"github.com/kai/codingjudge/internal/queue"
	"github.com/kai/codingjudge/internal/store"
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

	if cfg.QueueMode == config.QueueRedisStreams {
		client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		defer client.Close()
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
		slog.Info("outbox relay enabled", "relay_id", relayID)
	} else {
		slog.Warn("memory mode has no cross-process judge relay")
	}

	server := &http.Server{
		Addr: cfg.APIAddr,
		Handler: httpapi.AccessLog(httpapi.NewServer(st,
			httpapi.WithHTTPMetrics(metricsApp),
			httpapi.WithSubmissionMetrics(metricsApp),
			httpapi.WithMetricsHandler(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})),
		), slog.Default()),
		ReadHeaderTimeout: 5 * time.Second,
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
