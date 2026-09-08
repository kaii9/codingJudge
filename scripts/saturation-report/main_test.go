package main

import (
	"strings"
	"testing"
)

func TestRenderReportsScalingWithoutClaimingUniversalCapacity(t *testing.T) {
	metadata := map[string]string{
		"date": "2026-09-08T00:00:00Z", "git_commit": "abc123", "git_tracked_tree": "clean",
		"os": "Darwin", "arch": "arm64", "logical_cpus": "10", "memory": "16 GB",
		"docker_version": "27", "batch_size": "60", "language": "python", "vus": "20",
		"problem_id": "echo", "worker_concurrency": "1", "repetitions": "1",
	}
	rows := []benchmarkRow{
		{Trial: 1, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 60, Throughput: 1, PeakLag: 59},
		{Trial: 1, Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 30, Throughput: 2, PeakLag: 58},
		{Trial: 1, Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 15, Throughput: 4, PeakLag: 56},
	}
	got := render(metadata, rows)
	for _, want := range []string{"4.00x", "100.0% scaling efficiency", "not a universal production QPS limit", "make load-saturation"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q:\n%s", want, got)
		}
	}
}

func TestRenderDoesNotMisrepresentRegressionAsScaling(t *testing.T) {
	metadata := map[string]string{
		"batch_size": "60", "language": "python", "vus": "20", "problem_id": "echo",
		"worker_concurrency": "1", "repetitions": "1",
	}
	rows := []benchmarkRow{
		{Trial: 1, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 18, Throughput: 3.33, PeakLag: 58},
		{Trial: 1, Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 28, Throughput: 2.14, PeakLag: 58},
		{Trial: 1, Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 21, Throughput: 2.86, PeakLag: 56},
	}
	got := render(metadata, rows)
	for _, want := range []string{"regressed", "do not demonstrate horizontal scaling", "does not prove the cause", "Repeated trials"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q:\n%s", want, got)
		}
	}
}

func TestRenderCallsOutLowEfficiencyGain(t *testing.T) {
	metadata := map[string]string{"repetitions": "1"}
	rows := []benchmarkRow{
		{Trial: 1, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 20, Throughput: 3, PeakLag: 58},
		{Trial: 1, Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 18, Throughput: 3.3, PeakLag: 58},
		{Trial: 1, Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 17, Throughput: 3.5, PeakLag: 56},
	}
	got := render(metadata, rows)
	for _, want := range []string{"only 1.17x", "far below proportional scaling", "shared bottleneck"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q:\n%s", want, got)
		}
	}
}

func TestValidateRowsRejectsNonSaturatingRun(t *testing.T) {
	rows := []benchmarkRow{
		{Trial: 1, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 60, Throughput: 1, PeakLag: 59, PeakOutstanding: 60},
		{Trial: 1, Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 30, Throughput: 2, PeakLag: 0, PeakOutstanding: 60},
		{Trial: 1, Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 15, Throughput: 4, PeakLag: 56, PeakOutstanding: 60},
	}
	if err := validateRows(rows); err == nil || !strings.Contains(err.Error(), "positive Stream lag") {
		t.Fatalf("validateRows() error = %v, want positive Stream lag error", err)
	}
}

func TestAggregateUsesMedianAcrossRepeatedRuns(t *testing.T) {
	rows := []benchmarkRow{
		{Trial: 1, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 20, Throughput: 3, HTTPP95MS: 100, PeakOutstanding: 58},
		{Trial: 2, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 15, Throughput: 4, HTTPP95MS: 300, PeakOutstanding: 59},
		{Trial: 3, Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 30, Throughput: 2, HTTPP95MS: 200, PeakOutstanding: 57},
	}
	got := aggregate(rows)[0]
	if got.MedianMakespan != 20 || got.MedianThroughput != 3 || got.MinThroughput != 2 || got.MaxThroughput != 4 || got.MedianHTTPP95 != 200 || got.MaxPeakOutstanding != 59 {
		t.Fatalf("aggregate() = %+v", got)
	}
}
