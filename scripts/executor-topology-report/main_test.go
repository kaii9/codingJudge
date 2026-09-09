package main

import (
	"strings"
	"testing"
)

func TestValidateRowsAcceptsCompleteControlledTrial(t *testing.T) {
	if err := validateRows(completeTrial(1)); err != nil {
		t.Fatalf("validateRows() error = %v", err)
	}
}

func TestValidateRowsRejectsMissingTopology(t *testing.T) {
	rows := completeTrial(1)[:2]
	if err := validateRows(rows); err == nil || !strings.Contains(err.Error(), "complete single/shared/independent trials") {
		t.Fatalf("validateRows() error = %v, want incomplete trial error", err)
	}
}

func TestValidateRowsRejectsTopologyIdentityMismatch(t *testing.T) {
	rows := completeTrial(1)
	rows[1].Daemons = 2
	if err := validateRows(rows); err == nil || !strings.Contains(err.Error(), "shared topology") {
		t.Fatalf("validateRows() error = %v, want shared topology error", err)
	}
}

func TestValidateRowsRejectsMissingExecutorDistribution(t *testing.T) {
	rows := completeTrial(1)
	rows[2].ExecutorABatches = 60
	rows[2].ExecutorBBatches = 0
	if err := validateRows(rows); err == nil || !strings.Contains(err.Error(), "both executors") {
		t.Fatalf("validateRows() error = %v, want executor distribution error", err)
	}
}

func TestValidateRowsRejectsInvalidEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*benchmarkRow)
		want   string
	}{
		{name: "not all accepted", mutate: func(row *benchmarkRow) { row.Accepted-- }, want: "accepted"},
		{name: "no queue pressure", mutate: func(row *benchmarkRow) { row.PeakLag = 0 }, want: "positive Stream lag"},
		{name: "batch count mismatch", mutate: func(row *benchmarkRow) { row.ExecutorABatches-- }, want: "executor batches"},
		{name: "mean utilization over one", mutate: func(row *benchmarkRow) { row.MeanSlotUtilization = 1.01 }, want: "utilization"},
		{name: "no observed slot use", mutate: func(row *benchmarkRow) {
			row.MeanSlotUtilization = 0
			row.PeakSlotUtilization = 0
		}, want: "positive slot utilization"},
		{name: "peak below mean", mutate: func(row *benchmarkRow) { row.PeakSlotUtilization = 0.2 }, want: "peak utilization"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := completeTrial(1)
			tt.mutate(&rows[0])
			if err := validateRows(rows); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateRows() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAggregateUsesMediansAndPreservesTopologyOrder(t *testing.T) {
	rows := append(completeTrial(1), completeTrial(2)...)
	rows = append(rows, completeTrial(3)...)
	for index := range rows {
		switch rows[index].Trial {
		case 1:
			rows[index].Throughput *= 0.5
		case 3:
			rows[index].Throughput *= 2
		}
	}

	got := aggregate(rows)
	if len(got) != 3 || got[0].Topology != "single" || got[1].Topology != "shared" || got[2].Topology != "independent" {
		t.Fatalf("aggregate topology order = %+v", got)
	}
	if got[2].MedianThroughput != 5.4 || got[2].MinThroughput != 2.7 || got[2].MaxThroughput != 10.8 {
		t.Fatalf("independent aggregate = %+v", got[2])
	}
	if got[2].Accepted != 180 || got[2].Runs != 3 {
		t.Fatalf("independent accepted/runs = %+v", got[2])
	}
}

func TestRenderExplainsControlledComparisonAndLimits(t *testing.T) {
	metadata := map[string]string{
		"date": "2026-09-09T00:00:00Z", "git_commit": "abc123", "git_tracked_tree": "clean",
		"os": "Darwin", "arch": "arm64", "logical_cpus": "10", "memory": "16 GB",
		"docker_version": "27.5.1", "dind_image": "docker:27.5.1-dind", "batch_size": "60",
		"language": "python", "problem_id": "echo", "vus": "20", "workers": "4",
		"worker_concurrency": "1", "repetitions": "1", "problem_time_limit_ms": "10000",
	}
	got := render(metadata, completeTrial(1))
	for _, want := range []string{
		"# Executor Topology Saturation Benchmark",
		"1.80x versus the shared-daemon topology",
		"2.70x versus the single-executor baseline",
		"same Docker Desktop VM and host resources",
		"earliest submission creation to the latest terminal update",
		"temporary 10000 ms problem limit",
		"make load-executor-topologies",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q:\n%s", want, got)
		}
	}
}

func completeTrial(trial int) []benchmarkRow {
	return []benchmarkRow{
		{
			Trial: trial, Topology: "single", Executors: 1, Daemons: 1, TotalSlots: 1,
			Batch: 60, Accepted: 60, MakespanSeconds: 30, Throughput: 2,
			HTTPP95MS: 100, PeakPending: 2, PeakLag: 58, PeakOutstanding: 60,
			ExecutorQueueP95MS: 500, MeanSlotUtilization: 0.7, PeakSlotUtilization: 1,
			ExecutorABatches: 60,
		},
		{
			Trial: trial, Topology: "shared", Executors: 2, Daemons: 1, TotalSlots: 2,
			Batch: 60, Accepted: 60, MakespanSeconds: 20, Throughput: 3,
			HTTPP95MS: 100, PeakPending: 4, PeakLag: 56, PeakOutstanding: 60,
			ExecutorQueueP95MS: 250, MeanSlotUtilization: 0.65, PeakSlotUtilization: 1,
			ExecutorABatches: 30, ExecutorBBatches: 30,
		},
		{
			Trial: trial, Topology: "independent", Executors: 2, Daemons: 2, TotalSlots: 2,
			Batch: 60, Accepted: 60, MakespanSeconds: 11.111111, Throughput: 5.4,
			HTTPP95MS: 100, PeakPending: 4, PeakLag: 56, PeakOutstanding: 60,
			ExecutorQueueP95MS: 100, MeanSlotUtilization: 0.8, PeakSlotUtilization: 1,
			ExecutorABatches: 30, ExecutorBBatches: 30,
		},
	}
}
