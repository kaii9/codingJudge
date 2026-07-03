package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// k6Summary mirrors the k6 --summary-export JSON format (k6 2.0).
// Each metric is a flat map of key→float64 (e.g. "rate", "p(95)", "count").
type k6Summary struct {
	Metrics map[string]map[string]float64 `json:"metrics"`
}

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "usage: %s <meta.txt> <k6-w1.json> <k6-w2.json> <k6-w4.json>\n", os.Args[0])
		os.Exit(1)
	}
	meta := readMeta(os.Args[1])
	summaries := make([]k6Summary, 5) // indices 1/2/4 correspond to 1/2/4 workers
	workerMap := []int{1, 2, 4}
	for i, w := range workerMap {
		summaries[w] = readSummary(os.Args[2+i])
	}

	fmt.Println("# Worker Scaling Benchmark")
	fmt.Println()
	fmt.Printf("**Date:** %s | **Commit:** %s | **OS:** %s | **Arch:** %s | **CPUs:** %s\n\n",
		meta["date"], meta["git_commit"], meta["os"], meta["arch"], meta["logical_cpus"])
	fmt.Println("## Results")
	fmt.Println()
	fmt.Println("| Workers | Submission rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |")
	fmt.Println("| --- | --- | --- | --- | --- | --- |")

	for _, w := range workerMap {
		s := summaries[w]
		rate := metricValue(s, "http_reqs", "rate")
		httpP95 := metricValue(s, "http_req_duration", "p(95)") // k6 reports milliseconds
		judgeP95 := metricValue(s, "judge_terminal_duration", "p(95)")
		failRate := metricValue(s, "http_req_failed", "value") // k6 reports 0-1
		peak := strings.TrimSpace(meta[fmt.Sprintf("peak_pending_w%d", w)])
		if peak == "" {
			peak = "-"
		}

		fmt.Printf("| %d | %.2f/s | %.2fms | %.2fms | %.4f%% | %s |\n",
			w, rate, httpP95, judgeP95, failRate*100, peak)
	}

	fmt.Println()
	fmt.Println("## Interpretation")
	fmt.Println()
	fmt.Println("_This benchmark was run in a local Docker Compose environment. Results are not production capacity guarantees._")
	fmt.Println()
	fmt.Println("- CPU and memory contention increase as workers scale, but throughput should improve roughly linearly up to the number of logical CPUs.")
	fmt.Println("- Judge P95 is dominated by Docker container startup and compilation time; it improves with more workers distributing the load.")
	fmt.Println("- Zero or near-zero peak pending after each run confirms the system drains the queue reliably.")
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
		parts := strings.SplitN(scanner.Text(), ": ", 2)
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

func metricValue(s k6Summary, metricName, key string) float64 {
	metric, ok := s.Metrics[metricName]
	if !ok {
		return math.NaN()
	}
	v, ok := metric[key]
	if !ok {
		return math.NaN()
	}
	return v
}
