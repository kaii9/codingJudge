package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	APIAddr                      string
	DatabaseURL                  string
	RedisAddr                    string
	StorageMode                  StorageMode
	QueueMode                    QueueMode
	MinIOEndpoint                string
	MinIOAccessKey               string
	MinIOSecretKey               string
	MinIOBucket                  string
	MinIOUseSSL                  bool
	CookieSecure                 bool
	SubmissionRateLimitPerMinute int
	SubmissionRateLimitBurst     int
}

func Load(getenv func(string) string) Config {
	cfg := Config{
		APIAddr:                      ":8080",
		SubmissionRateLimitPerMinute: 10,
		SubmissionRateLimitBurst:     3,
	}
	if value := getenv("API_ADDR"); value != "" {
		cfg.APIAddr = value
	}
	cfg.DatabaseURL = getenv("DATABASE_URL")
	cfg.RedisAddr = getenv("REDIS_ADDR")
	cfg.MinIOEndpoint = getenv("MINIO_ENDPOINT")
	cfg.MinIOAccessKey = getenv("MINIO_ACCESS_KEY")
	cfg.MinIOSecretKey = getenv("MINIO_SECRET_KEY")
	cfg.MinIOBucket = getenv("MINIO_BUCKET")
	if cfg.MinIOBucket == "" {
		cfg.MinIOBucket = "codingjudge-assets"
	}
	if value := getenv("MINIO_USE_SSL"); value != "" {
		cfg.MinIOUseSSL, _ = strconv.ParseBool(value)
	}
	if value := getenv("COOKIE_SECURE"); value != "" {
		cfg.CookieSecure, _ = strconv.ParseBool(value)
	}
	if value := getenv("SUBMISSION_RATE_LIMIT_PER_MINUTE"); value != "" {
		cfg.SubmissionRateLimitPerMinute, _ = strconv.Atoi(value)
	}
	if value := getenv("SUBMISSION_RATE_LIMIT_BURST"); value != "" {
		cfg.SubmissionRateLimitBurst, _ = strconv.Atoi(value)
	}

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
	MinIOEndpoint     string
	MinIOAccessKey    string
	MinIOSecretKey    string
	MinIOBucket       string
	MinIOUseSSL       bool
	ExecutorURLs      []string
	ExecutorToken     string
	ExecutorTimeout   time.Duration
}

type ExecutorConfig struct {
	Addr           string
	Token          string
	MaxConcurrency int
	ShutdownGrace  time.Duration
	JudgeWorkdir   string
	JudgeImage     string
}

func ValidateAPI(cfg Config) error {
	if (cfg.DatabaseURL == "") != (cfg.RedisAddr == "") {
		return fmt.Errorf("DATABASE_URL and REDIS_ADDR must be configured together")
	}
	if err := validateMinIOValues(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey); err != nil {
		return err
	}
	if cfg.SubmissionRateLimitPerMinute < 1 {
		return fmt.Errorf("SUBMISSION_RATE_LIMIT_PER_MINUTE must be a positive integer")
	}
	if cfg.SubmissionRateLimitBurst < 1 {
		return fmt.Errorf("SUBMISSION_RATE_LIMIT_BURST must be a positive integer")
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
		MinIOEndpoint:     getenv("MINIO_ENDPOINT"),
		MinIOAccessKey:    getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey:    getenv("MINIO_SECRET_KEY"),
		MinIOBucket:       getenv("MINIO_BUCKET"),
		ExecutorToken:     getenv("EXECUTOR_TOKEN"),
		ExecutorTimeout:   2 * time.Minute,
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
	if cfg.ExecutorTimeout, err = parsePositiveDuration(getenv("EXECUTOR_TIMEOUT"), cfg.ExecutorTimeout); err != nil {
		return WorkerConfig{}, fmt.Errorf("EXECUTOR_TIMEOUT: %w", err)
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
	if cfg.MetricsAddr != "" && !isValidAddr(cfg.MetricsAddr) {
		return WorkerConfig{}, fmt.Errorf("WORKER_METRICS_ADDR %q is not a valid listen address", cfg.MetricsAddr)
	}
	if cfg.MinIOBucket == "" {
		cfg.MinIOBucket = "codingjudge-assets"
	}
	if value := getenv("MINIO_USE_SSL"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return WorkerConfig{}, fmt.Errorf("MINIO_USE_SSL: must be a boolean")
		}
		cfg.MinIOUseSSL = parsed
	}
	if err := validateMinIOValues(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey); err != nil {
		return WorkerConfig{}, err
	}
	executorURLs := getenv("EXECUTOR_URLS")
	if executorURLs == "" {
		executorURLs = getenv("EXECUTOR_URL")
	}
	if executorURLs != "" {
		seen := make(map[string]struct{})
		for _, rawURL := range strings.Split(executorURLs, ",") {
			executorURL := strings.TrimSpace(rawURL)
			if executorURL == "" {
				return WorkerConfig{}, fmt.Errorf("EXECUTOR_URLS must not contain empty entries")
			}
			parsed, err := url.Parse(executorURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return WorkerConfig{}, fmt.Errorf("executor URL %q must be an absolute http or https URL", executorURL)
			}
			normalized := strings.TrimRight(executorURL, "/")
			if _, duplicate := seen[normalized]; duplicate {
				return WorkerConfig{}, fmt.Errorf("duplicate executor URL %q", normalized)
			}
			seen[normalized] = struct{}{}
			cfg.ExecutorURLs = append(cfg.ExecutorURLs, normalized)
		}
	}
	if (len(cfg.ExecutorURLs) == 0) != (cfg.ExecutorToken == "") {
		return WorkerConfig{}, fmt.Errorf("EXECUTOR_URL or EXECUTOR_URLS and EXECUTOR_TOKEN must be configured together")
	}
	return cfg, nil
}

func LoadExecutor(getenv func(string) string) (ExecutorConfig, error) {
	cfg := ExecutorConfig{
		Addr:           ":8090",
		Token:          getenv("EXECUTOR_TOKEN"),
		MaxConcurrency: 2,
		ShutdownGrace:  30 * time.Second,
		JudgeWorkdir:   getenv("JUDGE_WORKDIR"),
		JudgeImage:     getenv("JUDGE_IMAGE"),
	}
	if value := getenv("EXECUTOR_ADDR"); value != "" {
		cfg.Addr = value
	}
	if cfg.Token == "" {
		return ExecutorConfig{}, fmt.Errorf("EXECUTOR_TOKEN is required")
	}
	if !isValidAddr(cfg.Addr) {
		return ExecutorConfig{}, fmt.Errorf("EXECUTOR_ADDR %q is not a valid listen address", cfg.Addr)
	}
	var err error
	if cfg.MaxConcurrency, err = parsePositiveInt(getenv("EXECUTOR_MAX_CONCURRENCY"), cfg.MaxConcurrency); err != nil {
		return ExecutorConfig{}, fmt.Errorf("EXECUTOR_MAX_CONCURRENCY: %w", err)
	}
	if cfg.ShutdownGrace, err = parsePositiveDuration(getenv("EXECUTOR_SHUTDOWN_GRACE"), cfg.ShutdownGrace); err != nil {
		return ExecutorConfig{}, fmt.Errorf("EXECUTOR_SHUTDOWN_GRACE: %w", err)
	}
	return cfg, nil
}

func validateMinIOValues(endpoint, accessKey, secretKey string) error {
	configured := 0
	for _, value := range []string{endpoint, accessKey, secretKey} {
		if value != "" {
			configured++
		}
	}
	if configured != 0 && configured != 3 {
		return fmt.Errorf("MINIO_ENDPOINT, MINIO_ACCESS_KEY and MINIO_SECRET_KEY must be configured together")
	}
	return nil
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

func isValidAddr(addr string) bool {
	// Must be a valid listen address for net.Listen: ":port" or "host:port".
	_, _, err := net.SplitHostPort(addr)
	return err == nil
}
