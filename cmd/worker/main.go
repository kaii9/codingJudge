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
	"github.com/kai/codingjudge/internal/queue"
	"github.com/kai/codingjudge/internal/store"
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

	st, err := store.NewPostgresStore(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("worker store setup failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer client.Close()

	service := judge.NewService(judge.NewDockerRunnerWithWorkDir(cfg.JudgeImage, cfg.JudgeWorkdir))
	slots := make([]judgeworker.Slot, 0, cfg.Concurrency)
	for index := 0; index < cfg.Concurrency; index++ {
		consumerID := fmt.Sprintf("%s-slot-%d", cfg.WorkerID, index+1)
		q := queue.NewRedisStreamsQueue(client, queue.DefaultJudgeStream, queue.DefaultJudgeGroup, consumerID)
		if err := q.Init(ctx); err != nil {
			slog.Error("worker queue setup failed", "consumer_id", consumerID, "error", err)
			os.Exit(1)
		}
		slots = append(slots, judgeworker.NewProcessor(st, q, service, judgeworker.Config{
			WorkerID:          consumerID,
			LeaseDuration:     cfg.LeaseDuration,
			HeartbeatInterval: cfg.HeartbeatInterval,
			MaxAttempts:       cfg.MaxAttempts,
		}))
	}

	slog.Info("judge worker started", "worker_id", cfg.WorkerID, "concurrency", cfg.Concurrency)
	if err := judgeworker.NewPool(slots, cfg.ShutdownGrace).Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("judge worker stopped", "error", err)
		os.Exit(1)
	}
}
