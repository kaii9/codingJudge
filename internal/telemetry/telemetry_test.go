package telemetry

import (
	"context"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
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

func TestInitExportsActualOTLPWithInstanceIdentity(t *testing.T) {
	for _, configured := range []string{"executor-a", ""} {
		t.Run("instance="+configured, func(t *testing.T) {
			requests := make(chan *collector.ExportTraceServiceRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/traces" {
					http.Error(w, "unexpected path", http.StatusNotFound)
					return
				}
				data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				request := new(collector.ExportTraceServiceRequest)
				if err != nil || proto.Unmarshal(data, request) != nil {
					http.Error(w, "invalid OTLP", http.StatusBadRequest)
					return
				}
				requests <- request
				w.Header().Set("Content-Type", "application/x-protobuf")
			}))
			defer server.Close()
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", server.URL)
			t.Setenv("EXECUTOR_INSTANCE_ID", configured)
			previous := otel.GetTracerProvider()
			defer otel.SetTracerProvider(previous)
			shutdown, err := Init("gojudge-executor")
			if err != nil {
				t.Fatal(err)
			}
			_, span := Start(context.Background(), "export-test")
			span.End()
			shutdown()
			select {
			case request := <-requests:
				if len(request.ResourceSpans) != 1 {
					t.Fatalf("resource count=%d", len(request.ResourceSpans))
				}
				rs := request.ResourceSpans[0]
				attributes := map[string]string{}
				for _, a := range rs.Resource.Attributes {
					attributes[a.Key] = a.Value.GetStringValue()
				}
				want := configured
				if want == "" {
					want, err = os.Hostname()
					if err != nil {
						t.Fatal(err)
					}
				}
				if attributes["service.name"] != "gojudge-executor" || attributes["service.instance.id"] != want {
					t.Fatalf("exported resource=%v, want instance=%s", attributes, want)
				}
				if len(rs.ScopeSpans) != 1 || len(rs.ScopeSpans[0].Spans) != 1 || rs.ScopeSpans[0].Spans[0].Name != "export-test" {
					t.Fatalf("missing actual span: %v", rs)
				}
			case <-time.After(time.Second):
				t.Fatal("no OTLP request reached collector")
			}
		})
	}
}
