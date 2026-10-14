package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Report struct {
	TraceID    string   `json:"traceId"`
	Spans      int      `json:"spans"`
	Services   []string `json:"services"`
	Attempts   []string `json:"attempts"`
	Instances  []string `json:"instances"`
	ErrorSpans int      `json:"errorSpans"`
}

type value struct {
	String  string          `json:"stringValue"`
	Integer json.RawMessage `json:"intValue"`
}
type attribute struct {
	Key   string
	Value value
}
type span struct {
	TraceID           string `json:"traceId"`
	ID                string `json:"spanId"`
	Parent            string `json:"parentSpanId"`
	Name              string
	Attributes        []attribute
	Status            struct{ Code json.RawMessage }
	service, instance string
}
type envelope struct {
	Error  json.RawMessage
	Result struct {
		ResourceSpans []struct {
			Resource   struct{ Attributes []attribute }
			ScopeSpans []struct{ Spans []span }
		}
	}
}

func canonicalID(raw string, size int) (string, error) {
	data, err := hex.DecodeString(raw)
	if err != nil || len(data) != size {
		data, err = base64.StdEncoding.DecodeString(raw)
	}
	if err != nil || len(data) != size || strings.Trim(hex.EncodeToString(data), "0") == "" {
		return "", fmt.Errorf("invalid %d-byte ID", size)
	}
	return hex.EncodeToString(data), nil
}

func verify(input io.Reader, traceID string, minAttempts int) (Report, error) {
	wanted, err := canonicalID(traceID, 16)
	if err != nil || len(traceID) != 32 || minAttempts < 1 {
		return Report{}, fmt.Errorf("require 32-hex trace ID and positive min-attempts")
	}
	limited := &io.LimitedReader{R: input, N: 16 << 20}
	decoder := json.NewDecoder(limited)
	spans := map[string]span{}
	for {
		var chunk envelope
		if err := decoder.Decode(&chunk); err == io.EOF {
			break
		} else if err != nil {
			return Report{}, fmt.Errorf("decode trace: %w", err)
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return Report{}, fmt.Errorf("Jaeger gateway returned an error")
		}
		for _, rs := range chunk.Result.ResourceSpans {
			service, instance := "", ""
			for _, a := range rs.Resource.Attributes {
				if a.Key == "service.name" {
					service = a.Value.String
				}
				if a.Key == "service.instance.id" {
					instance = a.Value.String
				}
			}
			for _, scope := range rs.ScopeSpans {
				for _, item := range scope.Spans {
					id, e := canonicalID(item.TraceID, 16)
					if e != nil || id != wanted {
						return Report{}, fmt.Errorf("span has invalid or different trace ID")
					}
					item.ID, e = canonicalID(item.ID, 8)
					if e != nil {
						return Report{}, e
					}
					if item.Parent != "" {
						item.Parent, e = canonicalID(item.Parent, 8)
						if e != nil {
							return Report{}, e
						}
					}
					if _, found := spans[item.ID]; found {
						return Report{}, fmt.Errorf("duplicate span ID")
					}
					if service == "" || instance == "" {
						return Report{}, fmt.Errorf("missing service or instance identity")
					}
					item.service, item.instance = service, instance
					spans[item.ID] = item
				}
			}
		}
	}
	if limited.N == 0 {
		return Report{}, fmt.Errorf("trace exceeds 16 MiB")
	}
	if len(spans) == 0 {
		return Report{}, fmt.Errorf("trace has no spans")
	}
	services, instances, attempts := map[string]bool{}, map[string]bool{}, map[string]bool{}
	roots := map[string]bool{}
	report := Report{TraceID: wanted, Spans: len(spans)}
	for _, item := range spans {
		services[item.service] = true
		instances[item.instance] = true
		seen := map[string]bool{}
		current := item
		for {
			if seen[current.ID] {
				return Report{}, fmt.Errorf("span parent cycle")
			}
			seen[current.ID] = true
			parent, found := spans[current.Parent]
			if !found {
				if current.service != "gojudge-platform" {
					return Report{}, fmt.Errorf("broken parent chain outside platform root")
				}
				roots[current.ID] = true
				break
			}
			current = parent
		}
		status := strings.Trim(string(item.Status.Code), "\"")
		if status == "2" || status == "STATUS_CODE_ERROR" {
			report.ErrorSpans++
		}
		if item.service == "gojudge-worker" && item.Name == "judge.process" {
			id, number := "", 0
			for _, a := range item.Attributes {
				if a.Key == "gojudge.execution_id" {
					id = a.Value.String
				}
				if a.Key == "gojudge.attempt" {
					number, _ = strconv.Atoi(strings.Trim(string(a.Value.Integer), "\""))
				}
			}
			if id != "" && number > 0 && strings.HasSuffix(id, fmt.Sprintf(":attempt-%d", number)) {
				attempts[id] = true
			}
		}
	}
	if len(roots) != 1 {
		return Report{}, fmt.Errorf("trace is not one connected tree")
	}
	for _, name := range []string{"gojudge-platform", "gojudge-judge-api", "gojudge-worker", "gojudge-executor"} {
		if !services[name] {
			return Report{}, fmt.Errorf("missing service %s", name)
		}
	}
	if len(attempts) < minAttempts {
		return Report{}, fmt.Errorf("attempts=%d, need %d", len(attempts), minAttempts)
	}
	for key := range services {
		report.Services = append(report.Services, key)
	}
	sort.Strings(report.Services)
	for key := range instances {
		report.Instances = append(report.Instances, key)
	}
	sort.Strings(report.Instances)
	for key := range attempts {
		report.Attempts = append(report.Attempts, key)
	}
	sort.Strings(report.Attempts)
	return report, nil
}

func main() {
	traceID := flag.String("trace-id", "", "32-hex trace ID")
	minimum := flag.Int("min-attempts", 1, "minimum acquired worker attempts")
	flag.Parse()
	report, err := verify(os.Stdin, *traceID, *minimum)
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(report)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
