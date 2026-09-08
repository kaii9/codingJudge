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
		{Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 60, Throughput: 1, PeakLag: 59},
		{Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 30, Throughput: 2, PeakLag: 58},
		{Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 15, Throughput: 4, PeakLag: 56},
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
		{Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 18, Throughput: 3.33, PeakLag: 58},
		{Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 28, Throughput: 2.14, PeakLag: 58},
		{Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 21, Throughput: 2.86, PeakLag: 56},
	}
	got := render(metadata, rows)
	for _, want := range []string{"regressed", "does not demonstrate horizontal scaling", "does not prove the cause", "sampled once"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q:\n%s", want, got)
		}
	}
}

func TestValidateRowsRejectsNonSaturatingRun(t *testing.T) {
	rows := []benchmarkRow{
		{Workers: 1, Batch: 60, Accepted: 60, MakespanSeconds: 60, Throughput: 1, PeakLag: 59, PeakOutstanding: 60},
		{Workers: 2, Batch: 60, Accepted: 60, MakespanSeconds: 30, Throughput: 2, PeakLag: 0, PeakOutstanding: 60},
		{Workers: 4, Batch: 60, Accepted: 60, MakespanSeconds: 15, Throughput: 4, PeakLag: 56, PeakOutstanding: 60},
	}
	if err := validateRows(rows); err == nil || !strings.Contains(err.Error(), "positive Stream lag") {
		t.Fatalf("validateRows() error = %v, want positive Stream lag error", err)
	}
}
