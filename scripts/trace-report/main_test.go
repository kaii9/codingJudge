package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

const testTrace = "1234567890abcdef1234567890abcdef"

func fixture(secondAttempt bool) map[string]any {
	services := []string{"gojudge-platform", "gojudge-judge-api", "gojudge-worker", "gojudge-executor"}
	resources := []any{}
	for i, service := range services {
		spanID := []string{"0000000000000001", "0000000000000002", "0000000000000003", "0000000000000004"}[i]
		parentID := []string{"9999999999999999", "0000000000000001", "0000000000000002", "0000000000000003"}[i]
		name := "rpc"
		attributes := []any{}
		if i == 2 {
			name = "judge.process"
			attributes = append(attributes, map[string]any{"key": "gojudge.execution_id", "value": map[string]any{"stringValue": "sub-demo:attempt-1"}}, map[string]any{"key": "gojudge.attempt", "value": map[string]any{"intValue": "1"}})
		}
		span := map[string]any{"traceId": testTrace, "spanId": spanID, "parentSpanId": parentID, "name": name, "attributes": attributes}
		spans := []any{span}
		if i == 2 && secondAttempt {
			span["status"] = map[string]any{"code": "STATUS_CODE_ERROR"}
			spans = append(spans, map[string]any{"traceId": testTrace, "spanId": "0000000000000005", "parentSpanId": "0000000000000002", "name": "judge.process", "attributes": []any{map[string]any{"key": "gojudge.execution_id", "value": map[string]any{"stringValue": "sub-demo:attempt-2"}}, map[string]any{"key": "gojudge.attempt", "value": map[string]any{"intValue": "2"}}}})
		}
		resources = append(resources, map[string]any{"resource": map[string]any{"attributes": []any{map[string]any{"key": "service.name", "value": map[string]any{"stringValue": service}}, map[string]any{"key": "service.instance.id", "value": map[string]any{"stringValue": service + "-a"}}}}, "scopeSpans": []any{map[string]any{"spans": spans}}})
	}
	return map[string]any{"result": map[string]any{"resourceSpans": resources}}
}

func spansIn(f map[string]any, i int) []any {
	return f["result"].(map[string]any)["resourceSpans"].([]any)[i].(map[string]any)["scopeSpans"].([]any)[0].(map[string]any)["spans"].([]any)
}
func encoded(t *testing.T, f any) []byte {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVerifyConnectedTraceAndRecoveryAttempts(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		minimum := 1
		if multiple {
			minimum = 2
		}
		report, err := verify(bytes.NewReader(encoded(t, fixture(multiple))), testTrace, minimum)
		if err != nil {
			t.Fatal(err)
		}
		if report.TraceID != testTrace || len(report.Services) != 4 || len(report.Attempts) != minimum || len(report.Instances) != 4 {
			t.Fatalf("report=%+v", report)
		}
		if multiple && report.ErrorSpans != 1 {
			t.Fatalf("missing failed attempt: %+v", report)
		}
	}
}

func TestVerifyRejectsMisleadingTraceEvidence(t *testing.T) {
	for _, problem := range []string{"missing-service", "wrong-trace", "broken-parent", "duplicate-span", "no-attempt", "cycle", "invalid-id", "empty"} {
		t.Run(problem, func(t *testing.T) {
			f := fixture(false)
			switch problem {
			case "missing-service":
				f["result"].(map[string]any)["resourceSpans"] = f["result"].(map[string]any)["resourceSpans"].([]any)[:3]
			case "wrong-trace":
				spansIn(f, 3)[0].(map[string]any)["traceId"] = strings.Repeat("a", 32)
			case "broken-parent":
				spansIn(f, 3)[0].(map[string]any)["parentSpanId"] = "8888888888888888"
			case "duplicate-span":
				spansIn(f, 3)[0].(map[string]any)["spanId"] = "0000000000000001"
			case "no-attempt":
				spansIn(f, 2)[0].(map[string]any)["attributes"] = []any{}
			case "cycle":
				spansIn(f, 0)[0].(map[string]any)["parentSpanId"] = "0000000000000004"
			case "invalid-id":
				spansIn(f, 3)[0].(map[string]any)["spanId"] = "oops"
			case "empty":
				f["result"] = map[string]any{}
			}
			if _, err := verify(bytes.NewReader(encoded(t, f)), testTrace, 1); err == nil {
				t.Fatalf("accepted %s", problem)
			}
		})
	}
	if _, err := verify(bytes.NewReader(encoded(t, fixture(false))), testTrace, 2); err == nil {
		t.Fatal("accepted insufficient attempts")
	}
	if _, err := verify(strings.NewReader(`{"error":{"httpCode":404,"message":"not found"}}`), testTrace, 1); err == nil {
		t.Fatal("accepted gateway error")
	}
}

func TestVerifyAcceptsGatewayBase64IDsAndNDJSON(t *testing.T) {
	f := fixture(false)
	for i := 0; i < 4; i++ {
		for _, item := range spansIn(f, i) {
			span := item.(map[string]any)
			for _, key := range []string{"traceId", "spanId", "parentSpanId"} {
				b, _ := hex.DecodeString(span[key].(string))
				span[key] = base64.StdEncoding.EncodeToString(b)
			}
		}
	}
	resources := f["result"].(map[string]any)["resourceSpans"].([]any)
	left := map[string]any{"result": map[string]any{"resourceSpans": resources[:2]}}
	right := map[string]any{"result": map[string]any{"resourceSpans": resources[2:]}}
	data := append(append(encoded(t, left), '\n'), encoded(t, right)...)
	if _, err := verify(bytes.NewReader(data), testTrace, 1); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyDoesNotCountDifferentSubmissionsAsRecovery(t *testing.T) {
	f := fixture(true)
	second := spansIn(f, 2)[1].(map[string]any)
	attributes := second["attributes"].([]any)
	attributes[0].(map[string]any)["value"] = map[string]any{"stringValue": "sub-other:attempt-2"}
	if _, err := verify(bytes.NewReader(encoded(t, f)), testTrace, 2); err == nil {
		t.Fatal("different submissions counted as recovery attempts")
	}
}
