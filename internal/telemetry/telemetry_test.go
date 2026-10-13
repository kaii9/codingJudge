package telemetry

import (
	"context"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestQueueTraceContextKeepsParentTraceAndNewAttemptSpan(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	defer provider.Shutdown(context.Background())
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)
	ctx, parent := otel.Tracer("test").Start(context.Background(), "submit")
	raw := TraceParent(ctx)
	if raw == "" {
		t.Fatal("empty persisted trace context")
	}
	consumed := Extract(context.Background(), raw)
	childCtx, child := otel.Tracer("test").Start(consumed, "attempt")
	defer child.End()
	defer parent.End()
	original := trace.SpanContextFromContext(ctx)
	next := trace.SpanContextFromContext(childCtx)
	if next.TraceID() != original.TraceID() || next.SpanID() == original.SpanID() {
		t.Fatalf("not parented: %v -> %v", original, next)
	}
}
