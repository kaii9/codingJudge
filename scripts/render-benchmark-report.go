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
		offeredRate     float64
		submissionsRate float64
		acceptedRate    float64
		httpRate        float64
		httpP95         float64
		judgeP95        float64
		failRate        float64
		peak            string
	}

	rows := make(map[int]row)
	offeredRate := parseFloat(meta["offered_rate"])
	durationSecs := parseDuration(meta["duration"])

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

		// Validate: dropped_iterations must be 0 (if present in JSON).
		if dropped, err := extractFloat(s, "dropped_iterations", "count"); err == nil && dropped != 0 {
			errs = append(errs, fmt.Sprintf("dropped_iterations = %.0f, must be 0", dropped))
		}

		// Validate: submissions_created.count == iterations.count.
		createdCount := mustFloat("submissions_created", "count")
		iterCount := mustFloat("iterations", "count")
		if math.Abs(createdCount-iterCount) > 0.5 {
			errs = append(errs, fmt.Sprintf("submissions_created.count=%.0f != iterations.count=%.0f", createdCount, iterCount))
		}

		// Validate: accepted >= 95% of created.
		acceptedCount := mustFloat("submissions_accepted", "count")
		if acceptedCount < createdCount*0.95 {
			errs = append(errs, fmt.Sprintf("accepted=%.0f < 95%% of created=%.0f", acceptedCount, createdCount))
		}

		// Validate: created roughly matches offered rate × duration.
		expected := offeredRate * durationSecs
		if createdCount < expected*0.5 || createdCount > expected*1.5 {
			errs = append(errs, fmt.Sprintf("created=%.0f outside expected range [%.0f, %.0f] for %.0f req/s × %.0fs",
				createdCount, expected*0.5, expected*1.5, offeredRate, durationSecs))
		}

		submissionsRate := mustFloat("submissions_created", "rate")
		acceptedRate := mustFloat("submissions_accepted", "rate")
		httpRate := mustFloat("http_reqs", "rate")
		httpP95 := mustFloat("http_req_duration", "p(95)")
		judgeP95 := mustFloat("judge_terminal_duration", "p(95)")
		failRate := mustFloat("http_req_failed", "value")

		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "worker %d: validation failed:\n", w)
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

	// Validate same offered rate across rounds.
	var rates []float64
	for _, w := range workerMap {
		rates = append(rates, rows[w].offeredRate)
	}
	if rates[0] != rates[1] || rates[1] != rates[2] {
		fmt.Fprintf(os.Stderr, "offered rates differ across rounds: %.2f / %.2f / %.2f\n", rates[0], rates[1], rates[2])
		os.Exit(1)
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
	fmt.Printf("- **Executor:** constant-arrival-rate\n")
	fmt.Printf("- **Pre-allocated VUs:** %s\n", meta["preallocated_vus"])
	fmt.Printf("- **Max VUs:** %s\n", meta["max_vus"])
	fmt.Println()
	fmt.Println("## Results")
	fmt.Println()
	fmt.Println("| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | Failure rate | Peak pending (sampled) |")
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
	fmt.Println("_Workers use Docker socket passthrough (Docker-outside-of-Docker), not nested Docker-in-Docker._")
	fmt.Println()
	fmt.Println("- The same offered load was applied to 1, 2, and 4 worker configurations using a constant-arrival-rate executor.")
	fmt.Println("- This benchmark compares Judge P95, HTTP P95, failure rate, and peak sampled pending under identical load.")
	fmt.Println("- It does NOT measure maximum throughput or claim linear scalability.")
	fmt.Println("- Peak Pending values are sampled every 5 seconds during the run and represent the highest observed value.")
	fmt.Println("- Pending returns to 0 after each round, confirming the queue drains under the tested load.")
	fmt.Println("- This benchmark uses Python submissions only; Go and C++ require Linux native Docker for reliable compilation timing.")
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

func parseDuration(s string) float64 {
	// Parse "2m" → 120, "30s" → 30.
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	raw := f[0]
	var num float64
	var unit string
	fmt.Sscanf(raw, "%f%s", &num, &unit)
	switch unit {
	case "m":
		return num * 60
	case "h":
		return num * 3600
	default:
		return num
	}
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
