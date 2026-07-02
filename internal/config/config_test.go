package config_test

import (
	"strings"
	"testing"
	"time"

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
}

func TestLoadWorkerUsesReliableDefaults(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"DATABASE_URL": "postgres://db",
		"REDIS_ADDR":   "redis:6379",
		"HOSTNAME":     "judge-1",
	}
	cfg, err := config.LoadWorker(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.WorkerID, "judge-1-") || cfg.Concurrency != 1 || cfg.LeaseDuration != 30*time.Second || cfg.HeartbeatInterval != 10*time.Second || cfg.MaxAttempts != 3 || cfg.ShutdownGrace != 30*time.Second {
		t.Fatalf("worker config = %+v", cfg)
	}
}

func TestLoadWorkerRejectsHeartbeatNotShorterThanLease(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"DATABASE_URL":             "postgres://db",
		"REDIS_ADDR":               "redis:6379",
		"JUDGE_LEASE_DURATION":     "10s",
		"JUDGE_HEARTBEAT_INTERVAL": "10s",
	}
	if _, err := config.LoadWorker(func(key string) string { return values[key] }); err == nil {
		t.Fatal("LoadWorker should reject heartbeat >= lease")
	}
}

func TestValidateAPIRejectsPartialDurableConfiguration(t *testing.T) {
	t.Parallel()
	cfg := config.Load(func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://db"
		}
		return ""
	})
	if err := config.ValidateAPI(cfg); err == nil {
		t.Fatal("ValidateAPI should reject PostgreSQL without Redis")
	}
}
