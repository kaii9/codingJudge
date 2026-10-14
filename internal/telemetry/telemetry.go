package telemetry

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"log/slog"
	"net/http"
	"os"
	"time"
)

var propagator = propagation.TraceContext{}

func Init(service string) (func(), error) {
	instance := os.Getenv("EXECUTOR_INSTANCE_ID")
	if instance == "" {
		var err error
		instance, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service), attribute.String("service.instance.id", instance))), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample()))}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(endpoint))
		if err != nil {
			return nil, err
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagator)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			slog.Warn("trace flush failed", "error", err)
		}
	}, nil
}
func TraceParent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	return carrier["traceparent"]
}
func Extract(ctx context.Context, parent string) context.Context {
	return propagator.Extract(ctx, propagation.MapCarrier{"traceparent": parent})
}
func Start(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("gojudge").Start(ctx, name, options...)
}
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := Start(ctx, "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func ClientUnary(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx, span := Start(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	for k, v := range carrier {
		md.Set(k, v)
	}
	if id := ExecutionID(ctx); id != "" {
		md.Set("execution-id", id)
	}
	err := invoke(metadata.NewOutgoingContext(ctx, md), method, req, reply, conn, opts...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "RPC failed")
	}
	return err
}
func ServerUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if parent := md.Get("traceparent"); len(parent) > 0 {
		ctx = Extract(ctx, parent[0])
	}
	ctx, span := Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	if id := md.Get("execution-id"); len(id) > 0 {
		span.SetAttributes(attribute.String("gojudge.execution_id", id[0]))
	}
	result, err := next(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "RPC failed")
	}
	return result, err
}

type executionKey struct{}

func WithExecutionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionKey{}, id)
}
func ExecutionID(ctx context.Context) string { id, _ := ctx.Value(executionKey{}).(string); return id }
