package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type benchmarkRow struct {
	Trial           int
	Workers         int
	Batch           int
	Accepted        int
	MakespanSeconds float64
	Throughput      float64
	HTTPP95MS       float64
	PeakPending     int
	PeakLag         int
	PeakOutstanding int
}

type aggregateRow struct {
	Workers            int
	Runs               int
	Batch              int
	Accepted           int
	MedianMakespan     float64
	MedianThroughput   float64
	MinThroughput      float64
	MaxThroughput      float64
	MedianHTTPP95      float64
	MaxPeakOutstanding int
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: saturation-report META_FILE RESULTS_CSV")
		os.Exit(2)
	}
	metadata, err := readMetadata(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rows, err := readRows(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if raw := metadata["repetitions"]; raw != "" {
		repetitions, err := strconv.Atoi(raw)
		if err != nil || repetitions != len(rows)/3 {
			fmt.Fprintf(os.Stderr, "metadata repetitions %q does not match %d runs per worker count\n", raw, len(rows)/3)
			os.Exit(1)
		}
	}
	fmt.Print(render(metadata, rows))
}

func readMetadata(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	metadata := make(map[string]string)
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			metadata[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return metadata, nil
}

func readRows(path string) ([]benchmarkRow, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	if _, err := reader.Read(); err != nil {
		return nil, err
	}
	var rows []benchmarkRow
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) != 10 {
			return nil, fmt.Errorf("expected 10 columns, got %d", len(record))
		}
		row := benchmarkRow{}
		if row.Trial, err = strconv.Atoi(record[0]); err != nil {
			return nil, err
		}
		if row.Workers, err = strconv.Atoi(record[1]); err != nil {
			return nil, err
		}
		if row.Batch, err = strconv.Atoi(record[2]); err != nil {
			return nil, err
		}
		if row.Accepted, err = strconv.Atoi(record[3]); err != nil {
			return nil, err
		}
		if row.MakespanSeconds, err = strconv.ParseFloat(record[4], 64); err != nil {
			return nil, err
		}
		if row.Throughput, err = strconv.ParseFloat(record[5], 64); err != nil {
			return nil, err
		}
		if row.HTTPP95MS, err = strconv.ParseFloat(record[6], 64); err != nil {
			return nil, err
		}
		if row.PeakPending, err = strconv.Atoi(record[7]); err != nil {
			return nil, err
		}
		if row.PeakLag, err = strconv.Atoi(record[8]); err != nil {
			return nil, err
		}
		if row.PeakOutstanding, err = strconv.Atoi(record[9]); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no benchmark rows")
	}
	if err := validateRows(rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func validateRows(rows []benchmarkRow) error {
	wantWorkers := []int{1, 2, 4}
	if len(rows) < len(wantWorkers) || len(rows)%len(wantWorkers) != 0 {
		return fmt.Errorf("expected an equal positive number of rows for workers 1/2/4, got %d rows", len(rows))
	}
	batch := rows[0].Batch
	counts := make(map[int]int, len(wantWorkers))
	seen := make(map[[2]int]bool, len(rows))
	for i, row := range rows {
		if row.Workers != 1 && row.Workers != 2 && row.Workers != 4 {
			return fmt.Errorf("row %d has unsupported worker count %d", i+1, row.Workers)
		}
		if row.Trial < 1 {
			return fmt.Errorf("row %d trial must be positive", i+1)
		}
		key := [2]int{row.Trial, row.Workers}
		if seen[key] {
			return fmt.Errorf("duplicate trial %d for %d workers", row.Trial, row.Workers)
		}
		seen[key] = true
		counts[row.Workers]++
		if batch <= 0 || row.Batch != batch {
			return fmt.Errorf("row %d batch = %d, want consistent positive batch %d", i+1, row.Batch, batch)
		}
		if row.Accepted != row.Batch {
			return fmt.Errorf("row %d accepted = %d, want %d", i+1, row.Accepted, row.Batch)
		}
		if row.MakespanSeconds <= 0 || row.Throughput <= 0 {
			return fmt.Errorf("row %d has non-positive makespan or throughput", i+1)
		}
		if row.PeakLag <= 0 {
			return fmt.Errorf("row %d did not observe positive Stream lag", i+1)
		}
		if row.PeakOutstanding < row.PeakPending || row.PeakOutstanding < row.PeakLag {
			return fmt.Errorf("row %d peak outstanding is inconsistent", i+1)
		}
	}
	wantRuns := len(rows) / len(wantWorkers)
	for _, workers := range wantWorkers {
		if counts[workers] != wantRuns {
			return fmt.Errorf("workers %d has %d runs, want %d", workers, counts[workers], wantRuns)
		}
		for trial := 1; trial <= wantRuns; trial++ {
			if !seen[[2]int{trial, workers}] {
				return fmt.Errorf("workers %d is missing trial %d", workers, trial)
			}
		}
	}
	return nil
}

func aggregate(rows []benchmarkRow) []aggregateRow {
	grouped := map[int][]benchmarkRow{1: {}, 2: {}, 4: {}}
	for _, row := range rows {
		grouped[row.Workers] = append(grouped[row.Workers], row)
	}
	result := make([]aggregateRow, 0, 3)
	for _, workers := range []int{1, 2, 4} {
		group := grouped[workers]
		if len(group) == 0 {
			continue
		}
		makespans := make([]float64, 0, len(group))
		throughputs := make([]float64, 0, len(group))
		httpP95s := make([]float64, 0, len(group))
		agg := aggregateRow{Workers: workers, Runs: len(group), Batch: group[0].Batch}
		for _, row := range group {
			agg.Accepted += row.Accepted
			makespans = append(makespans, row.MakespanSeconds)
			throughputs = append(throughputs, row.Throughput)
			httpP95s = append(httpP95s, row.HTTPP95MS)
			if row.PeakOutstanding > agg.MaxPeakOutstanding {
				agg.MaxPeakOutstanding = row.PeakOutstanding
			}
		}
		agg.MedianMakespan = median(makespans)
		agg.MedianThroughput = median(throughputs)
		agg.MedianHTTPP95 = median(httpP95s)
		sort.Float64s(throughputs)
		agg.MinThroughput = throughputs[0]
		agg.MaxThroughput = throughputs[len(throughputs)-1]
		result = append(result, agg)
	}
	return result
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func render(metadata map[string]string, rows []benchmarkRow) string {
	var out strings.Builder
	aggregates := aggregate(rows)
	repetitions := metadata["repetitions"]
	if repetitions == "" {
		repetitions = "1"
	}
	fmt.Fprintln(&out, "# Worker Saturation and Backlog-Drain Benchmark")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "- **Date:** %s\n", metadata["date"])
	fmt.Fprintf(&out, "- **Git commit:** `%s` (%s tracked tree)\n", metadata["git_commit"], metadata["git_tracked_tree"])
	fmt.Fprintf(&out, "- **Host:** %s/%s, %s logical CPUs, %s memory\n", metadata["os"], metadata["arch"], metadata["logical_cpus"], metadata["memory"])
	fmt.Fprintf(&out, "- **Docker:** %s\n", metadata["docker_version"])
	fmt.Fprintf(&out, "- **Workload:** %s `%s` %s submissions, %s VUs, shared-iterations burst; worker concurrency %s\n",
		metadata["batch_size"], metadata["problem_id"], metadata["language"], metadata["vus"], metadata["worker_concurrency"])
	fmt.Fprintf(&out, "- **Samples per worker count:** %s\n", repetitions)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The burst intentionally creates Redis Stream lag before the workers drain it. Makespan is measured from the earliest submission creation to the latest terminal update for that isolated run ID.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Workers | Runs | Accepted | Median makespan | Median throughput | Throughput range | vs 1 worker | Scaling efficiency | Median HTTP P95 | Max backlog |")
	fmt.Fprintln(&out, "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	base := aggregates[0].MedianThroughput
	for _, row := range aggregates {
		ratio := row.MedianThroughput / base
		efficiency := ratio / float64(row.Workers) * 100
		fmt.Fprintf(&out, "| %d | %d | %d/%d | %.3fs | %.3f/s | %.3f–%.3f/s | %.2fx | %.1f%% | %.2fms | %d |\n",
			row.Workers, row.Runs, row.Accepted, row.Batch*row.Runs, row.MedianMakespan, row.MedianThroughput,
			row.MinThroughput, row.MaxThroughput, ratio, efficiency, row.MedianHTTPP95, row.MaxPeakOutstanding)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "### Individual runs")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Trial | Workers | Accepted | Makespan | Throughput | HTTP P95 | Peak Pending | Peak Lag | Peak backlog |")
	fmt.Fprintln(&out, "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	ordered := append([]benchmarkRow(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Trial == ordered[j].Trial {
			return ordered[i].Workers < ordered[j].Workers
		}
		return ordered[i].Trial < ordered[j].Trial
	})
	for _, row := range ordered {
		fmt.Fprintf(&out, "| %d | %d | %d/%d | %.3fs | %.3f/s | %.2fms | %d | %d | %d |\n",
			row.Trial, row.Workers, row.Accepted, row.Batch, row.MakespanSeconds, row.Throughput,
			row.HTTPP95MS, row.PeakPending, row.PeakLag, row.PeakOutstanding)
	}
	fmt.Fprintln(&out)
	last := aggregates[len(aggregates)-1]
	ratio := last.MedianThroughput / base
	efficiency := ratio / float64(last.Workers) * 100
	fmt.Fprintln(&out, "## Interpretation")
	fmt.Fprintln(&out)
	fmt.Fprint(&out, "Every run processed the same batch and reached terminal `accepted` with zero HTTP and logical failures. Peak Stream lag above zero demonstrates that the workers were presented with queued work rather than an underloaded steady state. Comparisons use median throughput. ")
	switch {
	case ratio >= 1.1:
		fmt.Fprintf(&out, "The %d-worker configuration delivered %.2fx the one-worker throughput (%.1f%% scaling efficiency).\n", last.Workers, ratio, efficiency)
	case ratio >= 0.9:
		fmt.Fprintf(&out, "The %d-worker result was effectively flat at %.2fx the one-worker throughput (%.1f%% scaling efficiency), so these runs do not demonstrate useful horizontal scaling.\n", last.Workers, ratio, efficiency)
	default:
		fmt.Fprintf(&out, "The %d-worker configuration regressed to %.2fx the one-worker throughput (%.1f%% scaling efficiency), so these runs do not demonstrate horizontal scaling. This is consistent with contention in a shared execution resource, such as concurrent sandbox launches through one Docker daemon, but this benchmark alone does not prove the cause.\n", last.Workers, ratio, efficiency)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "This measures backlog-drain capacity on the recorded machine, not a universal production QPS limit. Docker Desktop, host scheduling, image caching, PostgreSQL, Redis, and the selected Python workload all affect the result. Repeated trials reduce one-off noise but are still insufficient for production capacity planning.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Reproduce")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "```bash")
	fmt.Fprintln(&out, "make load-saturation")
	fmt.Fprintln(&out, "```")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The runner refuses to publish a report unless every round has the exact requested submission count, zero HTTP and logical failures, all submissions accepted, observed positive Stream lag, and a fully drained queue.")
	return out.String()
}
