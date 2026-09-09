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

const (
	topologySingle      = "single"
	topologyShared      = "shared"
	topologyIndependent = "independent"
)

var topologyOrder = []string{topologySingle, topologyShared, topologyIndependent}

type benchmarkRow struct {
	Trial               int
	Topology            string
	Executors           int
	Daemons             int
	TotalSlots          int
	Batch               int
	Accepted            int
	MakespanSeconds     float64
	Throughput          float64
	HTTPP95MS           float64
	PeakPending         int
	PeakLag             int
	PeakOutstanding     int
	ExecutorQueueP95MS  float64
	MeanSlotUtilization float64
	PeakSlotUtilization float64
	ExecutorABatches    int
	ExecutorBBatches    int
}

type aggregateRow struct {
	Topology                  string
	Executors                 int
	Daemons                   int
	TotalSlots                int
	Runs                      int
	Batch                     int
	Accepted                  int
	MedianMakespan            float64
	MedianThroughput          float64
	MinThroughput             float64
	MaxThroughput             float64
	MedianHTTPP95             float64
	MaxPeakOutstanding        int
	MedianExecutorQueueP95    float64
	MedianMeanSlotUtilization float64
	MaxPeakSlotUtilization    float64
}

type topologySpec struct {
	executors int
	daemons   int
	slots     int
}

var topologySpecs = map[string]topologySpec{
	topologySingle:      {executors: 1, daemons: 1, slots: 1},
	topologyShared:      {executors: 2, daemons: 1, slots: 2},
	topologyIndependent: {executors: 2, daemons: 2, slots: 2},
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: executor-topology-report META_FILE RESULTS_CSV")
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
		if err != nil || repetitions != len(rows)/len(topologyOrder) {
			fmt.Fprintf(os.Stderr, "metadata repetitions %q does not match %d complete trials\n", raw, len(rows)/len(topologyOrder))
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
		row, err := parseRow(record)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", len(rows)+1, err)
		}
		rows = append(rows, row)
	}
	if err := validateRows(rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func parseRow(record []string) (benchmarkRow, error) {
	if len(record) != 18 {
		return benchmarkRow{}, fmt.Errorf("expected 18 columns, got %d", len(record))
	}
	row := benchmarkRow{Topology: record[1]}
	intFields := []struct {
		column int
		target *int
	}{
		{0, &row.Trial}, {2, &row.Executors}, {3, &row.Daemons}, {4, &row.TotalSlots},
		{5, &row.Batch}, {6, &row.Accepted}, {10, &row.PeakPending}, {11, &row.PeakLag},
		{12, &row.PeakOutstanding}, {16, &row.ExecutorABatches}, {17, &row.ExecutorBBatches},
	}
	for _, field := range intFields {
		value, err := strconv.Atoi(record[field.column])
		if err != nil {
			return benchmarkRow{}, err
		}
		*field.target = value
	}
	floatFields := []struct {
		column int
		target *float64
	}{
		{7, &row.MakespanSeconds}, {8, &row.Throughput}, {9, &row.HTTPP95MS},
		{13, &row.ExecutorQueueP95MS}, {14, &row.MeanSlotUtilization}, {15, &row.PeakSlotUtilization},
	}
	for _, field := range floatFields {
		value, err := strconv.ParseFloat(record[field.column], 64)
		if err != nil {
			return benchmarkRow{}, err
		}
		*field.target = value
	}
	return row, nil
}

func validateRows(rows []benchmarkRow) error {
	if len(rows) == 0 || len(rows)%len(topologyOrder) != 0 {
		return fmt.Errorf("expected complete single/shared/independent trials, got %d rows", len(rows))
	}
	wantRuns := len(rows) / len(topologyOrder)
	batch := rows[0].Batch
	seen := make(map[string]bool, len(rows))
	counts := make(map[string]int, len(topologyOrder))
	for index, row := range rows {
		spec, ok := topologySpecs[row.Topology]
		if !ok {
			return fmt.Errorf("row %d has unsupported topology %q", index+1, row.Topology)
		}
		if row.Executors != spec.executors || row.Daemons != spec.daemons || row.TotalSlots != spec.slots {
			return fmt.Errorf("row %d %s topology has executors/daemons/slots %d/%d/%d, want %d/%d/%d",
				index+1, row.Topology, row.Executors, row.Daemons, row.TotalSlots, spec.executors, spec.daemons, spec.slots)
		}
		if row.Trial < 1 {
			return fmt.Errorf("row %d trial must be positive", index+1)
		}
		key := fmt.Sprintf("%d/%s", row.Trial, row.Topology)
		if seen[key] {
			return fmt.Errorf("duplicate topology %s in trial %d", row.Topology, row.Trial)
		}
		seen[key] = true
		counts[row.Topology]++
		if batch <= 0 || row.Batch != batch {
			return fmt.Errorf("row %d batch = %d, want consistent positive batch %d", index+1, row.Batch, batch)
		}
		if row.Accepted != row.Batch {
			return fmt.Errorf("row %d accepted = %d, want %d", index+1, row.Accepted, row.Batch)
		}
		if row.MakespanSeconds <= 0 || row.Throughput <= 0 || row.HTTPP95MS < 0 || row.ExecutorQueueP95MS < 0 {
			return fmt.Errorf("row %d has invalid duration or throughput metrics", index+1)
		}
		if row.PeakLag <= 0 {
			return fmt.Errorf("row %d did not observe positive Stream lag", index+1)
		}
		if row.PeakOutstanding < row.PeakPending || row.PeakOutstanding < row.PeakLag {
			return fmt.Errorf("row %d peak outstanding is inconsistent", index+1)
		}
		if row.MeanSlotUtilization < 0 || row.MeanSlotUtilization > 1 || row.PeakSlotUtilization < 0 || row.PeakSlotUtilization > 1 {
			return fmt.Errorf("row %d utilization must be between zero and one", index+1)
		}
		if row.PeakSlotUtilization < row.MeanSlotUtilization {
			return fmt.Errorf("row %d peak utilization %.3f is below mean utilization %.3f", index+1, row.PeakSlotUtilization, row.MeanSlotUtilization)
		}
		if row.PeakSlotUtilization <= 0 {
			return fmt.Errorf("row %d did not observe positive slot utilization", index+1)
		}
		if row.ExecutorABatches+row.ExecutorBBatches != row.Batch {
			return fmt.Errorf("row %d executor batches total %d, want %d", index+1, row.ExecutorABatches+row.ExecutorBBatches, row.Batch)
		}
		if row.ExecutorABatches <= 0 || (row.Executors == 1 && row.ExecutorBBatches != 0) {
			return fmt.Errorf("row %d executor distribution is invalid", index+1)
		}
		if row.Executors == 2 && row.ExecutorBBatches <= 0 {
			return fmt.Errorf("row %d did not use both executors", index+1)
		}
	}
	for _, topology := range topologyOrder {
		if counts[topology] != wantRuns {
			return fmt.Errorf("expected complete single/shared/independent trials: topology %s has %d runs, want %d", topology, counts[topology], wantRuns)
		}
		for trial := 1; trial <= wantRuns; trial++ {
			if !seen[fmt.Sprintf("%d/%s", trial, topology)] {
				return fmt.Errorf("expected complete single/shared/independent trials: missing %s trial %d", topology, trial)
			}
		}
	}
	return nil
}

func aggregate(rows []benchmarkRow) []aggregateRow {
	grouped := make(map[string][]benchmarkRow, len(topologyOrder))
	for _, row := range rows {
		grouped[row.Topology] = append(grouped[row.Topology], row)
	}
	result := make([]aggregateRow, 0, len(topologyOrder))
	for _, topology := range topologyOrder {
		group := grouped[topology]
		if len(group) == 0 {
			continue
		}
		makespans := make([]float64, 0, len(group))
		throughputs := make([]float64, 0, len(group))
		httpP95s := make([]float64, 0, len(group))
		queueP95s := make([]float64, 0, len(group))
		meanUtilizations := make([]float64, 0, len(group))
		agg := aggregateRow{
			Topology: topology, Executors: group[0].Executors, Daemons: group[0].Daemons,
			TotalSlots: group[0].TotalSlots, Runs: len(group), Batch: group[0].Batch,
		}
		for _, row := range group {
			agg.Accepted += row.Accepted
			makespans = append(makespans, row.MakespanSeconds)
			throughputs = append(throughputs, row.Throughput)
			httpP95s = append(httpP95s, row.HTTPP95MS)
			queueP95s = append(queueP95s, row.ExecutorQueueP95MS)
			meanUtilizations = append(meanUtilizations, row.MeanSlotUtilization)
			if row.PeakOutstanding > agg.MaxPeakOutstanding {
				agg.MaxPeakOutstanding = row.PeakOutstanding
			}
			if row.PeakSlotUtilization > agg.MaxPeakSlotUtilization {
				agg.MaxPeakSlotUtilization = row.PeakSlotUtilization
			}
		}
		agg.MedianMakespan = median(makespans)
		agg.MedianThroughput = median(throughputs)
		agg.MedianHTTPP95 = median(httpP95s)
		agg.MedianExecutorQueueP95 = median(queueP95s)
		agg.MedianMeanSlotUtilization = median(meanUtilizations)
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
	aggregates := aggregate(rows)
	byTopology := make(map[string]aggregateRow, len(aggregates))
	for _, row := range aggregates {
		byTopology[row.Topology] = row
	}
	single := byTopology[topologySingle]
	shared := byTopology[topologyShared]
	independent := byTopology[topologyIndependent]
	sharedVsSingle := shared.MedianThroughput / single.MedianThroughput
	independentVsShared := independent.MedianThroughput / shared.MedianThroughput
	independentVsSingle := independent.MedianThroughput / single.MedianThroughput

	var out strings.Builder
	fmt.Fprintln(&out, "# Executor Topology Saturation Benchmark")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Technical summary")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "The independent-daemon topology delivered %.2fx versus the shared-daemon topology and %.2fx versus the single-executor baseline on this recorded host. ", independentVsShared, independentVsSingle)
	fmt.Fprintf(&out, "Adding a second executor while retaining one daemon produced %.2fx the baseline throughput. These are descriptive local measurements, not production capacity claims.\n", sharedVsSingle)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## The controlled comparison isolates Docker daemon topology")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "All three configurations used the same DinD image, workload, worker count, runtime images, and host. `shared` and `independent` expose the same executor slot count, so their comparison changes the Docker daemon count without changing declared executor capacity.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Topology | Executors | Daemons | Slots | Runs | Accepted | Median makespan | Median throughput | Throughput range | vs single | Median HTTP P95 | Executor queue P95 | Mean slot utilization | Max backlog |")
	fmt.Fprintln(&out, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, row := range aggregates {
		fmt.Fprintf(&out, "| %s | %d | %d | %d | %d | %d/%d | %.3fs | %.3f/s | %.3f–%.3f/s | %.2fx | %.2fms | ≤ %.2fms | %.1f%% | %d |\n",
			row.Topology, row.Executors, row.Daemons, row.TotalSlots, row.Runs, row.Accepted, row.Batch*row.Runs,
			row.MedianMakespan, row.MedianThroughput, row.MinThroughput, row.MaxThroughput,
			row.MedianThroughput/single.MedianThroughput, row.MedianHTTPP95, row.MedianExecutorQueueP95,
			row.MedianMeanSlotUtilization*100, row.MaxPeakOutstanding)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The small categorical comparison is shown as an exact table rather than a chart: three topologies do not provide a meaningful trend, while exact throughput and topology invariants are central to auditing the result.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Individual runs show the observed variation")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Trial | Topology | Accepted | Makespan | Throughput | HTTP P95 | Peak Pending | Peak Lag | Peak backlog | Executor queue P95 | Mean/peak slot utilization | Executor A/B batches |")
	fmt.Fprintln(&out, "| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	ordered := append([]benchmarkRow(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Trial == ordered[j].Trial {
			return topologyIndex(ordered[i].Topology) < topologyIndex(ordered[j].Topology)
		}
		return ordered[i].Trial < ordered[j].Trial
	})
	for _, row := range ordered {
		fmt.Fprintf(&out, "| %d | %s | %d/%d | %.3fs | %.3f/s | %.2fms | %d | %d | %d | ≤ %.2fms | %.1f%% / %.1f%% | %d / %d |\n",
			row.Trial, row.Topology, row.Accepted, row.Batch, row.MakespanSeconds, row.Throughput,
			row.HTTPP95MS, row.PeakPending, row.PeakLag, row.PeakOutstanding, row.ExecutorQueueP95MS,
			row.MeanSlotUtilization*100, row.PeakSlotUtilization*100, row.ExecutorABatches, row.ExecutorBBatches)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Measurement definitions and method")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "- **Workload:** %s Python submissions to `%s`, submitted by %s k6 VUs; %s workers with concurrency %s.\n", metadata["batch_size"], metadata["problem_id"], metadata["vus"], metadata["workers"], metadata["worker_concurrency"])
	fmt.Fprintf(&out, "- **Correctness isolation:** the runner uses a temporary %s ms problem limit so container-startup latency remains visible in makespan without becoming a solution TLE, then restores the original limit.\n", metadata["problem_time_limit_ms"])
	fmt.Fprintln(&out, "- **Makespan:** elapsed database time from the earliest submission creation to the latest terminal update for one isolated run ID.")
	fmt.Fprintln(&out, "- **Throughput:** accepted batch size divided by makespan; comparisons use the median across balanced-order trials.")
	fmt.Fprintln(&out, "- **Executor queue P95:** the upper bound of the first Prometheus histogram bucket containing at least 95% of slot-wait observations during the round.")
	fmt.Fprintln(&out, "- **Slot utilization:** one-second Prometheus samples of total in-flight batches divided by configured slots; the mean uses samples where queue work or slot use was present.")
	fmt.Fprintln(&out, "- **Per-node concurrency:** one slot per executor. A preliminary four-slot shared-daemon run produced sandbox startup timeouts, so the controlled benchmark uses the highest tested shared-daemon setting that preserves accepted-result validity.")
	fmt.Fprintln(&out, "- **Validity gates:** exact requested submissions, zero HTTP/logical failures, all accepted, positive Stream lag and slot use, correct daemon identities, both executors used where declared, and zero final Pending/Lag.")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "Recorded at %s from commit `%s` (%s tracked tree) on %s/%s with %s logical CPUs and %s memory. Docker %s; runtime node image `%s`.\n",
		metadata["date"], metadata["git_commit"], metadata["git_tracked_tree"], metadata["os"], metadata["arch"],
		metadata["logical_cpus"], metadata["memory"], metadata["docker_version"], metadata["dind_image"])
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Limitations and robustness")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The two DinD daemons have separate Docker state and sandbox work volumes but still share the same Docker Desktop VM and host resources, including CPU and physical storage. The result therefore isolates daemon state better than the earlier shared-socket benchmark, but it does not measure two independent machines or prove a universal production QPS limit. Python avoids compile-stage variance; a mixed-language capacity test may rank the topologies differently. Balanced order and repeated medians reduce one-off cache and thermal effects but do not eliminate them.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Recommended next step")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "Repeat the same workload on two independent VM hosts before choosing production capacity, then implement health-aware executor removal and recovery so additional capacity also improves availability.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Reproduce")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "```bash")
	fmt.Fprintln(&out, "make load-executor-topologies")
	fmt.Fprintln(&out, "```")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The runner retains per-round k6 summaries, queue/utilization samples, topology evidence, and the validated CSV under `loadtest/results/`. It refuses to replace this report when any validity gate fails.")
	return out.String()
}

func topologyIndex(topology string) int {
	for index, candidate := range topologyOrder {
		if candidate == topology {
			return index
		}
	}
	return len(topologyOrder)
}
