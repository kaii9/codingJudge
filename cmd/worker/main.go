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

	"github.com/kai/codingjudge/internal/judge"
	"github.com/kai/codingjudge/internal/workerapi"
)

func main() {
	addr := getenv("WORKER_ADDR", ":8081")
	image := os.Getenv("JUDGE_IMAGE")
	workDir := os.Getenv("JUDGE_WORKDIR")

	service := judge.NewService(judge.NewDockerRunnerWithWorkDir(image, workDir))
	server := &http.Server{
		Addr:              addr,
		Handler:           workerapi.NewServer(service),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("worker listening", "addr", addr, "image", image, "work_dir", workDir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
