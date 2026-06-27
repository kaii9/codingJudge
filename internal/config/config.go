package config

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
	WorkerURL   string
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
	cfg.WorkerURL = getenv("WORKER_URL")
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
