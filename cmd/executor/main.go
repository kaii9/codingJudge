package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaii9/codingJudge/internal/config"
	"github.com/kaii9/codingJudge/internal/executor"
	"github.com/kaii9/codingJudge/internal/judge"
	"github.com/kaii9/codingJudge/internal/metrics"
	"github.com/kaii9/codingJudge/internal/rpcsecurity"
	"github.com/kaii9/codingJudge/internal/telemetry"
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
	shutdownTracing, err := telemetry.Init("gojudge-executor")
	if err != nil {
		slog.Error("tracing setup failed", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing()

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsApp := metrics.New(registry)
	owner := os.Getenv("EXECUTOR_INSTANCE_ID")
	if owner == "" {
		owner, _ = os.Hostname()
	}
	runner := judge.NewDockerRunnerWithWorkDir(cfg.JudgeImage, cfg.JudgeWorkdir, judge.WithSandboxMetrics(metricsApp), judge.WithSandboxOwner(owner))
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 10*time.Second)
	removed, cleanupErr := runner.CleanupOwnedSandboxes(cleanupCtx)
	cleanupCancel()
	if cleanupErr != nil {
		slog.Error("orphan sandbox cleanup failed", "error", cleanupErr)
		os.Exit(1)
	}
	if removed > 0 {
		slog.Info("removed orphan sandboxes", "count", removed, "executor_instance", owner)
	}
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
	rpcOptions, err := rpcsecurity.ServerOptions(os.Getenv)
	if err != nil {
		slog.Error("RPC TLS setup failed", "error", err)
		os.Exit(1)
	}
	if err := executor.Serve(ctx, cfg.Addr, cfg.GRPCAddr, executorServer, cfg.ShutdownGrace, rpcOptions...); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("executor stopped", "error", err)
		os.Exit(1)
	}
}
