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

	"github.com/kaii9/codingJudge/internal/config"
	"github.com/kaii9/codingJudge/internal/executor"
	"github.com/kaii9/codingJudge/internal/judge"
	"github.com/kaii9/codingJudge/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := config.LoadExecutor(os.Getenv)
	if err != nil {
		slog.Error("invalid executor configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsApp := metrics.New(registry)
	runner := judge.NewDockerRunnerWithWorkDir(cfg.JudgeImage, cfg.JudgeWorkdir, judge.WithSandboxMetrics(metricsApp))
	executorServer, err := executor.NewServer(
		runner,
		cfg.Token,
		cfg.MaxConcurrency,
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		runner.Ping,
		executor.WithPrometheusMetrics(registry, cfg.MaxConcurrency),
	)
	if err != nil {
		slog.Error("executor setup failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           executorServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("executor listening", "addr", cfg.Addr, "max_concurrency", cfg.MaxConcurrency)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("executor stopped", "error", err)
		os.Exit(1)
	}
}
