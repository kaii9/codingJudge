package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kai/codingjudge/internal/config"
	"github.com/kai/codingjudge/internal/judge"
	"github.com/kai/codingjudge/internal/judgeworker"
	"github.com/kai/codingjudge/internal/metrics"
	"github.com/kai/codingjudge/internal/queue"
	"github.com/kai/codingjudge/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.LoadWorker(os.Getenv)
	if err != nil {
		slog.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 可被子系统（如 metrics serve 失败）取消的 worker 上下文。
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	st, err := store.NewPostgresStore(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("worker store setup failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer client.Close()

	// 创建独立的 Prometheus 注册器并设置 worker slots 指标。
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsApp := metrics.New(registry)
	metricsApp.SetWorkerSlots(float64(cfg.Concurrency))

	service := judge.NewService(judge.NewDockerRunnerWithWorkDir(cfg.JudgeImage, cfg.JudgeWorkdir), judge.WithMetrics(metricsApp))
	slots := make([]judgeworker.Slot, 0, cfg.Concurrency)
	for index := 0; index < cfg.Concurrency; index++ {
		consumerID := fmt.Sprintf("%s-slot-%d", cfg.WorkerID, index+1)
		q := queue.NewRedisStreamsQueue(client, queue.DefaultJudgeStream, queue.DefaultJudgeGroup, consumerID, queue.WithMetrics(metricsApp))
		if err := q.Init(ctx); err != nil {
			slog.Error("worker queue setup failed", "consumer_id", consumerID, "error", err)
			os.Exit(1)
		}
		slots = append(slots, judgeworker.NewProcessor(st, q, service, judgeworker.Config{
			WorkerID:          consumerID,
			LeaseDuration:     cfg.LeaseDuration,
			HeartbeatInterval: cfg.HeartbeatInterval,
			MaxAttempts:       cfg.MaxAttempts,
			Metrics:           metricsApp,
		}))
	}

	slog.Info("judge worker started", "worker_id", cfg.WorkerID, "concurrency", cfg.Concurrency)

	// 在启动判题池之前同步绑定 metrics 端口，避免绑定失败时池已在运行。
	if cfg.MetricsAddr != "" {
		if err := metrics.Bind(ctx, cfg.MetricsAddr, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), cancelWorker); err != nil {
			slog.Error("worker metrics bind failed", "error", err)
			os.Exit(1)
		}
	}

	if err := judgeworker.NewPool(slots, cfg.ShutdownGrace).Run(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("judge worker stopped", "error", err)
		os.Exit(1)
	}
}
