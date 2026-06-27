package config_test

import (
	"testing"

	"github.com/kai/codingjudge/internal/config"
)

func TestLoadUsesMemoryDefaults(t *testing.T) {
	t.Parallel()

	cfg := config.Load(func(key string) string { return "" })

	if cfg.APIAddr != ":8080" {
		t.Fatalf("APIAddr = %q, want :8080", cfg.APIAddr)
	}
	if cfg.StorageMode != config.StorageMemory {
		t.Fatalf("StorageMode = %q, want %q", cfg.StorageMode, config.StorageMemory)
	}
	if cfg.QueueMode != config.QueueMemory {
		t.Fatalf("QueueMode = %q, want %q", cfg.QueueMode, config.QueueMemory)
	}
}

func TestLoadEnablesPostgresAndRedisWhenConfigured(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"API_ADDR":     ":9000",
		"DATABASE_URL": "postgres://codingjudge:secret@postgres:5432/codingjudge?sslmode=disable",
		"REDIS_ADDR":   "redis:6379",
		"WORKER_URL":   "http://worker:8081",
	}
	cfg := config.Load(func(key string) string { return values[key] })

	if cfg.APIAddr != ":9000" {
		t.Fatalf("APIAddr = %q, want :9000", cfg.APIAddr)
	}
	if cfg.StorageMode != config.StoragePostgres {
		t.Fatalf("StorageMode = %q, want %q", cfg.StorageMode, config.StoragePostgres)
	}
	if cfg.QueueMode != config.QueueRedisStreams {
		t.Fatalf("QueueMode = %q, want %q", cfg.QueueMode, config.QueueRedisStreams)
	}
	if cfg.WorkerURL != "http://worker:8081" {
		t.Fatalf("WorkerURL = %q", cfg.WorkerURL)
	}
}
