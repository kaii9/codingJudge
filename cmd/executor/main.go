package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
		executor.WithAdmission(cfg.MaxWaiters, cfg.WaitTimeout),
	)
	if err != nil {
		slog.Error("executor setup failed", "error", err)
		os.Exit(1)
	}

	slog.Info("executor listening", "addr", cfg.Addr, "grpc_addr", cfg.GRPCAddr, "max_concurrency", cfg.MaxConcurrency)
	if err := executor.Serve(ctx, cfg.Addr, cfg.GRPCAddr, executorServer, cfg.ShutdownGrace); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("executor stopped", "error", err)
		os.Exit(1)
	}
}
