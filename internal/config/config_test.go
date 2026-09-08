package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/config"
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
	if cfg.SubmissionRateLimitPerMinute != 10 || cfg.SubmissionRateLimitBurst != 3 {
		t.Fatalf("submission rate limit = %d/min burst %d, want 10/min burst 3", cfg.SubmissionRateLimitPerMinute, cfg.SubmissionRateLimitBurst)
	}
}

func TestLoadReadsSubmissionRateLimit(t *testing.T) {
	t.Parallel()
	cfg := config.Load(func(key string) string {
		switch key {
		case "SUBMISSION_RATE_LIMIT_PER_MINUTE":
			return "120"
		case "SUBMISSION_RATE_LIMIT_BURST":
			return "8"
		default:
			return ""
		}
	})
	if err := config.ValidateAPI(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SubmissionRateLimitPerMinute != 120 || cfg.SubmissionRateLimitBurst != 8 {
		t.Fatalf("submission rate limit = %d/min burst %d", cfg.SubmissionRateLimitPerMinute, cfg.SubmissionRateLimitBurst)
	}
}

func TestValidateAPIRejectsInvalidSubmissionRateLimit(t *testing.T) {
	t.Parallel()
	cfg := config.Load(func(key string) string {
		if key == "SUBMISSION_RATE_LIMIT_PER_MINUTE" {
			return "invalid"
		}
		return ""
	})
	if err := config.ValidateAPI(cfg); err == nil {
		t.Fatal("ValidateAPI should reject malformed submission rate limit")
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

func TestLoadReadsSecureCookieSetting(t *testing.T) {
	t.Parallel()

	cfg := config.Load(func(key string) string {
		if key == "COOKIE_SECURE" {
			return "true"
		}
		return ""
	})
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure = false, want true")
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

func TestWorkerMetricsAddrDefaultsAndOverrides(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "default", env: "", want: ":9091"},
		{name: "custom", env: ":9191", want: ":9191"},
		{name: "off", env: "off", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]string{
				"DATABASE_URL":        "postgres://db",
				"REDIS_ADDR":          "redis:6379",
				"WORKER_METRICS_ADDR": tt.env,
			}
			cfg, err := config.LoadWorker(func(key string) string { return values[key] })
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MetricsAddr != tt.want {
				t.Errorf("MetricsAddr = %q, want %q", cfg.MetricsAddr, tt.want)
			}
		})
	}
}

func TestLoadWorkerReadsMinIOConfig(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"DATABASE_URL":     "postgres://db",
		"REDIS_ADDR":       "redis:6379",
		"MINIO_ENDPOINT":   "minio:9000",
		"MINIO_ACCESS_KEY": "minioadmin",
		"MINIO_SECRET_KEY": "minioadmin",
		"MINIO_BUCKET":     "codingjudge-assets",
		"MINIO_USE_SSL":    "true",
	}
	cfg, err := config.LoadWorker(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinIOEndpoint != "minio:9000" || cfg.MinIOAccessKey != "minioadmin" || cfg.MinIOSecretKey != "minioadmin" || cfg.MinIOBucket != "codingjudge-assets" || !cfg.MinIOUseSSL {
		t.Fatalf("MinIO config = %+v", cfg)
	}
}

func TestLoadWorkerRejectsMalformedMinIOUseSSL(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"DATABASE_URL":  "postgres://db",
		"REDIS_ADDR":    "redis:6379",
		"MINIO_USE_SSL": "sometimes",
	}
	if _, err := config.LoadWorker(func(key string) string { return values[key] }); err == nil {
		t.Fatal("LoadWorker should reject malformed MINIO_USE_SSL")
	}
}

func TestLoadWorkerRejectsPartialMinIOConfig(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"DATABASE_URL":   "postgres://db",
		"REDIS_ADDR":     "redis:6379",
		"MINIO_ENDPOINT": "minio:9000",
	}
	if _, err := config.LoadWorker(func(key string) string { return values[key] }); err == nil {
		t.Fatal("LoadWorker should reject partial MinIO configuration")
	}
}

func TestLoadWorkerRejectsMalformedMetricsAddr(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"DATABASE_URL":        "postgres://db",
		"REDIS_ADDR":          "redis:6379",
		"WORKER_METRICS_ADDR": "not-a-valid-addr",
	}
	if _, err := config.LoadWorker(func(key string) string { return values[key] }); err == nil {
		t.Fatal("LoadWorker should reject malformed WORKER_METRICS_ADDR")
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

func TestValidateAPIRejectsPartialMinIOConfig(t *testing.T) {
	t.Parallel()
	cfg := config.Load(func(key string) string {
		switch key {
		case "MINIO_ENDPOINT":
			return "minio:9000"
		default:
			return ""
		}
	})
	if err := config.ValidateAPI(cfg); err == nil {
		t.Fatal("ValidateAPI should reject partial MinIO config")
	}
}
