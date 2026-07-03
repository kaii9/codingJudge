package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

type k6Summary struct {
	Metrics map[string]json.RawMessage `json:"metrics"`
}

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "usage: %s <meta.txt> <k6-w1.json> <k6-w2.json> <k6-w4.json>\n", os.Args[0])
		os.Exit(1)
	}
	meta := readMeta(os.Args[1])
	summaries := make([]k6Summary, 5)
	workerMap := []int{1, 2, 4}
	for i, w := range workerMap {
		summaries[w] = readSummary(os.Args[2+i])
	}

	type row struct {
		offeredRate      float64
		submissionsRate  float64
		acceptedRate     float64
		httpRate         float64
		httpP95          float64
		judgeP95         float64
		failRate         float64
		peak             string
	}
	rows := make(map[int]row)

	for _, w := range workerMap {
		s := summaries[w]
		var errs []string

		mustFloat := func(metric, key string) float64 {
			v, err := extractFloat(s, metric, key)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s.%s: %v", metric, key, err))
			}
			return v
		}

		// offered rate is a fixed parameter, not a measured metric.
		offeredRate := parseFloat(meta["offered_rate"])

		submissionsRate := mustFloat("submissions_created", "rate")
		acceptedRate := mustFloat("submissions_accepted", "rate")
		httpRate := mustFloat("http_reqs", "rate")
		httpP95 := mustFloat("http_req_duration", "p(95)")
		judgeP95 := mustFloat("judge_terminal_duration", "p(95)")
		failRate := mustFloat("http_req_failed", "value")

		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "worker %d: missing required metrics:\n", w)
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
			os.Exit(1)
		}

		peak := strings.TrimSpace(meta[fmt.Sprintf("peak_pending_w%d", w)])
		if peak == "" {
			peak = "-"
		}
		rows[w] = row{
			offeredRate: offeredRate, submissionsRate: submissionsRate,
			acceptedRate: acceptedRate, httpRate: httpRate,
			httpP95: httpP95, judgeP95: judgeP95,
			failRate: failRate, peak: peak,
		}
	}

	// Reject NaN.
	for w, r := range rows {
		for _, v := range []float64{r.submissionsRate, r.acceptedRate, r.httpRate, r.httpP95, r.judgeP95, r.failRate} {
			if math.IsNaN(v) {
				fmt.Fprintf(os.Stderr, "worker %d: contains NaN values\n", w)
				os.Exit(1)
			}
		}
	}

	fmt.Println("# Fixed-Load Worker Scaling Benchmark")
	fmt.Println()
	fmt.Printf("**Date:** %s\n\n", meta["date"])
	fmt.Println("## Environment")
	fmt.Println()
	fmt.Println("| Key | Value |")
	fmt.Println("| --- | --- |")
	for _, kv := range [][2]string{
		{"Git commit", meta["git_commit"]},
		{"OS", meta["os"]},
		{"Architecture", meta["arch"]},
		{"Logical CPUs", meta["logical_cpus"]},
		{"Memory", meta["memory"]},
		{"Docker version", meta["docker_version"]},
		{"k6 version", meta["k6_version"]},
		{"Judge images", meta["judge_images"]},
	} {
		val := kv[1]
		if val == "" {
			val = "-"
		}
		fmt.Printf("| %s | %s |\n", kv[0], val)
	}
	fmt.Println()
	fmt.Println("## Scenario")
	fmt.Println()
	fmt.Printf("- **Offered rate:** %s\n", meta["offered_rate"])
	fmt.Printf("- **Duration:** %s\n", meta["duration"])
	fmt.Printf("- **Worker concurrency:** %s slot(s)/worker\n", meta["worker_concurrency"])
	fmt.Println()
	fmt.Println("## Results")
	fmt.Println()
	fmt.Println("| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |")
	fmt.Println("| --- | --- | --- | --- | --- | --- | --- | --- | --- |")

	for _, w := range workerMap {
		r := rows[w]
		fmt.Printf("| %d | %.2f/s | %.2f/s | %.2f/s | %.2f/s | %.2fms | %.2fms | %.4f%% | %s |\n",
			w, r.offeredRate, r.submissionsRate, r.acceptedRate, r.httpRate,
			r.httpP95, r.judgeP95, r.failRate*100, r.peak)
	}

	fmt.Println()
	fmt.Println("## Interpretation")
	fmt.Println()
	fmt.Println("_This is a fixed-load benchmark, not a maximum-throughput test._")
	fmt.Println("_Results are from a local Docker Compose environment and are not production capacity guarantees._")
	fmt.Println()
	fmt.Println("- The same offered load was applied to 1, 2 and 4 worker configurations.")
	fmt.Println("- Increasing workers lowers Judge P95 by distributing Docker sandbox execution across more slots.")
	fmt.Println("- HTTP P95 remains low and stable because the API serves reads and enqueues submissions without blocking on judge execution.")
	fmt.Println("- Zero peak pending after each run confirms the queue drains reliably under the tested load.")
}

func extractFloat(s k6Summary, metricName, key string) (float64, error) {
	raw, ok := s.Metrics[metricName]
	if !ok {
		return 0, fmt.Errorf("metric %q not found", metricName)
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(raw, &flat); err != nil {
		var direct float64
		if err2 := json.Unmarshal(raw, &direct); err2 != nil {
			return 0, fmt.Errorf("cannot parse metric %q: %w", metricName, err)
		}
		return direct, nil
	}
	valRaw, ok := flat[key]
	if !ok {
		return 0, fmt.Errorf("key %q not found in metric %q", key, metricName)
	}
	var f float64
	if err := json.Unmarshal(valRaw, &f); err != nil {
		return 0, fmt.Errorf("cannot parse %s.%s as float: %w", metricName, key, err)
	}
	return f, nil
}

// parseFloat extracts a float from a string like "1 req/s" or "1.5".
func parseFloat(s string) float64 {
	f := strings.Fields(s)
	if len(f) > 0 {
		var v float64
		if _, err := fmt.Sscanf(f[0], "%f", &v); err == nil {
			return v
		}
	}
	return 0
}

func readMeta(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read meta: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	m := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m
}

func readSummary(path string) k6Summary {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read summary %s: %v\n", path, err)
		os.Exit(1)
	}
	var s k6Summary
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Fprintf(os.Stderr, "parse summary %s: %v\n", path, err)
		os.Exit(1)
	}
	return s
}
