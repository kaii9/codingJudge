package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type StorageMode string

const (
	StorageMemory   StorageMode = "memory"
	StoragePostgres StorageMode = "postgres"
)

type QueueMode string

const (
	QueueMemory       QueueMode = "memory"
	QueueRedisStreams QueueMode = "redis_streams"
)

type Config struct {
	APIAddr     string
	DatabaseURL string
	RedisAddr   string
	StorageMode StorageMode
	QueueMode   QueueMode
}

func Load(getenv func(string) string) Config {
	cfg := Config{
		APIAddr: ":8080",
	}
	if value := getenv("API_ADDR"); value != "" {
		cfg.APIAddr = value
	}
	cfg.DatabaseURL = getenv("DATABASE_URL")
	cfg.RedisAddr = getenv("REDIS_ADDR")

	if cfg.DatabaseURL != "" {
		cfg.StorageMode = StoragePostgres
	} else {
		cfg.StorageMode = StorageMemory
	}
	if cfg.RedisAddr != "" {
		cfg.QueueMode = QueueRedisStreams
	} else {
		cfg.QueueMode = QueueMemory
	}
	return cfg
}

type WorkerConfig struct {
	DatabaseURL       string
	RedisAddr         string
	WorkerID          string
	Concurrency       int
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	MaxAttempts       int
	ShutdownGrace     time.Duration
	JudgeWorkdir      string
	JudgeImage        string
	MetricsAddr       string
}

func ValidateAPI(cfg Config) error {
	if (cfg.DatabaseURL == "") != (cfg.RedisAddr == "") {
		return fmt.Errorf("DATABASE_URL and REDIS_ADDR must be configured together")
	}
	return nil
}

func LoadWorker(getenv func(string) string) (WorkerConfig, error) {
	cfg := WorkerConfig{
		DatabaseURL:       getenv("DATABASE_URL"),
		RedisAddr:         getenv("REDIS_ADDR"),
		WorkerID:          getenv("WORKER_ID"),
		Concurrency:       1,
		LeaseDuration:     30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		MaxAttempts:       3,
		ShutdownGrace:     30 * time.Second,
		JudgeWorkdir:      getenv("JUDGE_WORKDIR"),
		JudgeImage:        getenv("JUDGE_IMAGE"),
	}
	if cfg.DatabaseURL == "" || cfg.RedisAddr == "" {
		return WorkerConfig{}, fmt.Errorf("DATABASE_URL and REDIS_ADDR are required")
	}
	if cfg.WorkerID == "" {
		hostname := getenv("HOSTNAME")
		if hostname == "" {
			hostname, _ = os.Hostname()
		}
		cfg.WorkerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}
	var err error
	if cfg.Concurrency, err = parsePositiveInt(getenv("WORKER_CONCURRENCY"), cfg.Concurrency); err != nil {
		return WorkerConfig{}, fmt.Errorf("WORKER_CONCURRENCY: %w", err)
	}
	if cfg.MaxAttempts, err = parsePositiveInt(getenv("JUDGE_MAX_ATTEMPTS"), cfg.MaxAttempts); err != nil {
		return WorkerConfig{}, fmt.Errorf("JUDGE_MAX_ATTEMPTS: %w", err)
	}
	if cfg.LeaseDuration, err = parsePositiveDuration(getenv("JUDGE_LEASE_DURATION"), cfg.LeaseDuration); err != nil {
		return WorkerConfig{}, fmt.Errorf("JUDGE_LEASE_DURATION: %w", err)
	}
	if cfg.HeartbeatInterval, err = parsePositiveDuration(getenv("JUDGE_HEARTBEAT_INTERVAL"), cfg.HeartbeatInterval); err != nil {
		return WorkerConfig{}, fmt.Errorf("JUDGE_HEARTBEAT_INTERVAL: %w", err)
	}
	if cfg.ShutdownGrace, err = parsePositiveDuration(getenv("WORKER_SHUTDOWN_GRACE"), cfg.ShutdownGrace); err != nil {
		return WorkerConfig{}, fmt.Errorf("WORKER_SHUTDOWN_GRACE: %w", err)
	}
	if cfg.HeartbeatInterval >= cfg.LeaseDuration {
		return WorkerConfig{}, fmt.Errorf("heartbeat interval must be shorter than lease duration")
	}
	cfg.MetricsAddr = getenv("WORKER_METRICS_ADDR")
	if cfg.MetricsAddr == "" {
		cfg.MetricsAddr = ":9091"
	}
	if cfg.MetricsAddr == "off" {
		cfg.MetricsAddr = ""
	}
	return cfg, nil
}

func parsePositiveInt(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("must be a positive integer")
	}
	return parsed, nil
}

func parsePositiveDuration(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("must be a positive duration")
	}
	return parsed, nil
}
