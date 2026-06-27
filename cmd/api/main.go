package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kai/codingjudge/internal/config"
	"github.com/kai/codingjudge/internal/dispatcher"
	"github.com/kai/codingjudge/internal/httpapi"
	"github.com/kai/codingjudge/internal/problems"
	"github.com/kai/codingjudge/internal/queue"
	"github.com/kai/codingjudge/internal/store"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load(os.Getenv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, cleanupStore, err := buildStore(ctx, cfg)
	if err != nil {
		slog.Error("store setup failed", "error", err)
		os.Exit(1)
	}
	defer cleanupStore()

	q, err := buildQueue(ctx, cfg)
	if err != nil {
		slog.Error("queue setup failed", "error", err)
		os.Exit(1)
	}

	if cfg.WorkerURL != "" {
		client := dispatcher.NewHTTPJudgeClient(cfg.WorkerURL, &http.Client{Timeout: 30 * time.Second})
		go func() {
			if err := dispatcher.New(st, q, client).Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("dispatcher stopped", "error", err)
			}
		}()
		slog.Info("judge dispatcher enabled", "worker_url", cfg.WorkerURL)
	} else {
		slog.Warn("WORKER_URL is empty; submissions will remain queued")
	}

	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           httpapi.AccessLog(httpapi.NewServer(st, q), slog.Default()),
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
	dispatcher.Store
}

type appQueue interface {
	httpapi.JobQueue
	dispatcher.Queue
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

func buildQueue(ctx context.Context, cfg config.Config) (appQueue, error) {
	if cfg.QueueMode == config.QueueRedisStreams {
		client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		q := queue.NewRedisStreamsQueue(client, queue.DefaultJudgeStream, queue.DefaultJudgeGroup, "api-dispatcher")
		if err := q.Init(ctx); err != nil {
			client.Close()
			return nil, err
		}
		return q, nil
	}
	return queue.NewMemoryQueue(100), nil
}
