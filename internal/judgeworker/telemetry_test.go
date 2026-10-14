package judgeworker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judgeworker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestAttemptSpanIdentifiesSuccessfulAndFailedExecution(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "retry"}[failure], func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			defer otel.SetTracerProvider(previous)
			defer provider.Shutdown(context.Background())
			calls := []string{}
			st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, completeOK: true, releaseOK: true, calls: &calls}
			q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
			j := &fakeJudge{result: domain.JudgeResult{Status: domain.StatusAccepted}, calls: &calls}
			if failure {
				j.result = domain.JudgeResult{}
				j.err = errors.New("executor unavailable")
			}
			processor := judgeworker.NewProcessor(st, q, j, judgeworker.Config{HeartbeatInterval: time.Hour})
			err := processor.ProcessOne(context.Background())
			if (err != nil) != failure {
				t.Fatalf("error=%v, failure=%t", err, failure)
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 || spans[0].Name != "judge.process" {
				t.Fatalf("spans=%v", spans)
			}
			foundID, foundAttempt := false, false
			for _, a := range spans[0].Attributes {
				if a.Key == "gojudge.execution_id" && a.Value.AsString() == "sub-1:attempt-1" {
					foundID = true
				}
				if a.Key == "gojudge.attempt" && a.Value.AsInt64() == 1 {
					foundAttempt = true
				}
				if a.Key == "code" || a.Key == "token" {
					t.Fatalf("sensitive attribute=%v", a)
				}
			}
			if !foundID || !foundAttempt {
				t.Fatalf("missing attempt identity: %v", spans[0].Attributes)
			}
			if failure && (spans[0].Status.Code != codes.Error || len(spans[0].Events) == 0) {
				t.Fatalf("failed attempt not marked: %v", spans[0])
			}
		})
	}
}
