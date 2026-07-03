package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

// Bind listens on addr synchronously and starts serving in a goroutine.
// It returns any bind error BEFORE the caller proceeds, so a port conflict
// is detected before the worker pool starts.
func Bind(ctx context.Context, addr string, handler http.Handler) error {
	if handler == nil {
		return fmt.Errorf("metrics handler is nil")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("metrics listen %s: %w", addr, err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	go func() {
		slog.Info("metrics server listening", "addr", addr)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics serve failed", "error", err)
		}
	}()
	return nil
}

// Serve starts a metrics HTTP server on addr and blocks until the context is
// cancelled. Use Bind for non-blocking startup with synchronous error handling.
func Serve(ctx context.Context, addr string, handler http.Handler) error {
	if handler == nil {
		return fmt.Errorf("metrics handler is nil")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("metrics listen %s: %w", addr, err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	slog.Info("metrics server listening", "addr", addr)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics serve: %w", err)
	}
	return nil
}
