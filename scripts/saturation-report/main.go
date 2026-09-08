package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type benchmarkRow struct {
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
		if len(record) != 9 {
			return nil, fmt.Errorf("expected 9 columns, got %d", len(record))
		}
		row := benchmarkRow{}
		if row.Workers, err = strconv.Atoi(record[0]); err != nil {
			return nil, err
		}
		if row.Batch, err = strconv.Atoi(record[1]); err != nil {
			return nil, err
		}
		if row.Accepted, err = strconv.Atoi(record[2]); err != nil {
			return nil, err
		}
		if row.MakespanSeconds, err = strconv.ParseFloat(record[3], 64); err != nil {
			return nil, err
		}
		if row.Throughput, err = strconv.ParseFloat(record[4], 64); err != nil {
			return nil, err
		}
		if row.HTTPP95MS, err = strconv.ParseFloat(record[5], 64); err != nil {
			return nil, err
		}
		if row.PeakPending, err = strconv.Atoi(record[6]); err != nil {
			return nil, err
		}
		if row.PeakLag, err = strconv.Atoi(record[7]); err != nil {
			return nil, err
		}
		if row.PeakOutstanding, err = strconv.Atoi(record[8]); err != nil {
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
	if len(rows) != len(wantWorkers) {
		return fmt.Errorf("expected %d benchmark rows, got %d", len(wantWorkers), len(rows))
	}
	batch := rows[0].Batch
	for i, row := range rows {
		if row.Workers != wantWorkers[i] {
			return fmt.Errorf("row %d workers = %d, want %d", i+1, row.Workers, wantWorkers[i])
		}
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
	return nil
}

func render(metadata map[string]string, rows []benchmarkRow) string {
	var out strings.Builder
	fmt.Fprintln(&out, "# Worker Saturation and Backlog-Drain Benchmark")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "- **Date:** %s\n", metadata["date"])
	fmt.Fprintf(&out, "- **Git commit:** `%s` (%s tracked tree)\n", metadata["git_commit"], metadata["git_tracked_tree"])
	fmt.Fprintf(&out, "- **Host:** %s/%s, %s logical CPUs, %s memory\n", metadata["os"], metadata["arch"], metadata["logical_cpus"], metadata["memory"])
	fmt.Fprintf(&out, "- **Docker:** %s\n", metadata["docker_version"])
	fmt.Fprintf(&out, "- **Workload:** %s `%s` %s submissions, %s VUs, shared-iterations burst; worker concurrency %s\n",
		metadata["batch_size"], metadata["problem_id"], metadata["language"], metadata["vus"], metadata["worker_concurrency"])
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The burst intentionally creates Redis Stream lag before the workers drain it. Makespan is measured from the earliest submission creation to the latest terminal update for that isolated run ID.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Workers | Batch | Accepted | Drain makespan | Throughput | vs 1 worker | Scaling efficiency | HTTP P95 | Peak Pending | Peak Lag | Peak outstanding |")
	fmt.Fprintln(&out, "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	base := rows[0].Throughput
	for _, row := range rows {
		ratio := row.Throughput / base
		efficiency := ratio / float64(row.Workers) * 100
		fmt.Fprintf(&out, "| %d | %d | %d | %.3fs | %.3f/s | %.2fx | %.1f%% | %.2fms | %d | %d | %d |\n",
			row.Workers, row.Batch, row.Accepted, row.MakespanSeconds, row.Throughput, ratio, efficiency,
			row.HTTPP95MS, row.PeakPending, row.PeakLag, row.PeakOutstanding)
	}
	fmt.Fprintln(&out)
	last := rows[len(rows)-1]
	ratio := last.Throughput / base
	efficiency := ratio / float64(last.Workers) * 100
	fmt.Fprintln(&out, "## Interpretation")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "All rounds processed the same batch and reached terminal `accepted` with zero HTTP and logical failures. Peak Stream lag above zero demonstrates that the workers were presented with queued work rather than an underloaded steady state. The %d-worker run delivered %.2fx the one-worker throughput (%.1f%% scaling efficiency).\n", last.Workers, ratio, efficiency)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "This measures backlog-drain capacity on the recorded machine, not a universal production QPS limit. Docker Desktop, host scheduling, image caching, PostgreSQL, Redis, and the selected Python workload all affect the result.")
	return out.String()
}
