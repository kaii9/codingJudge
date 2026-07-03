package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

// Serve starts a metrics HTTP server on addr. It returns when the context is
// cancelled and logs bind errors. A nil handler means promhttp defaults are
// used by the caller (registered externally).
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
