package executor

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"time"
)

func Serve(ctx context.Context, httpAddr, grpcAddr string, core *Server, grace time.Duration, options ...grpc.ServerOption) error {
	httpListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return err
	}
	defer httpListener.Close()
	rpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	defer rpcListener.Close()
	rpc := NewGRPCServer(core, options...)
	httpServer := &http.Server{Handler: core.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	errs := make(chan error, 2)
	go func() { errs <- rpc.Serve(rpcListener) }()
	go func() { errs <- httpServer.Serve(httpListener) }()
	probeCtx, stopProbe := context.WithCancel(ctx)
	defer stopProbe()
	go func() {
		if core.readiness == nil {
			return
		}
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-probeCtx.Done():
				return
			case <-timer.C:
				checkCtx, cancel := context.WithTimeout(probeCtx, 2*time.Second)
				checkErr := core.readiness(checkCtx)
				cancel()
				if probeCtx.Err() != nil {
					return
				}
				core.SetServing(checkErr == nil)
				timer.Reset(2 * time.Second)
			}
		}
	}()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errs:
	}
	stopProbe()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := core.Drain(shutdownCtx); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = core.Drain(cleanupCtx)
		cleanupCancel()
	}
	rpcDone := make(chan struct{})
	go func() { rpc.GracefulStop(); close(rpcDone) }()
	_ = httpServer.Shutdown(shutdownCtx)
	select {
	case <-rpcDone:
	case <-shutdownCtx.Done():
		rpc.Stop()
		_ = httpServer.Close()
		<-rpcDone
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}
