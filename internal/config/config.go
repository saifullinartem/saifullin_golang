package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	OTelServiceName string
	OTelProtocol    string
	ServiceVersion  string

	HTTPAddr                string
	HTTPReadTimeout         time.Duration
	HTTPReadHeaderTimeout   time.Duration
	HTTPWriteTimeout        time.Duration
	HTTPIdleTimeout         time.Duration
	LogLevel                string
	ShutdownTimeout         time.Duration
	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
	PositionCheckInterval   time.Duration
	PositionStaleAfter      time.Duration
	PositionWorkers         int
	PositionQueueSize       int
	PositionScanBatchSize   int
	PushHTTPURL             string
	PushRequestTimeout      time.Duration
}

func Load() (Config, error) {
	var cfg Config
	var err error

	if cfg.HTTPAddr, err = required("HTTP_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = required("LOG_LEVEL"); err != nil {
		return Config{}, err
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
	if cfg.DatabaseURL, err = required("DATABASE_URL"); err != nil {
		return Config{}, err
	}

	if cfg.ShutdownTimeout, err = duration("SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"HTTP_READ_TIMEOUT", &cfg.HTTPReadTimeout},
		{"HTTP_READ_HEADER_TIMEOUT", &cfg.HTTPReadHeaderTimeout},
		{"HTTP_WRITE_TIMEOUT", &cfg.HTTPWriteTimeout},
		{"HTTP_IDLE_TIMEOUT", &cfg.HTTPIdleTimeout},
	} {
		if *setting.target, err = duration(setting.name); err != nil {
			return Config{}, err
		}
	}
	if cfg.DatabaseMaxConnLifetime, err = duration("DATABASE_MAX_CONN_LIFETIME"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseConnectTimeout, err = duration("DATABASE_CONNECT_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseQueryTimeout, err = duration("DATABASE_QUERY_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMaxConns, err = int32Value("DATABASE_MAX_CONNS"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMinConns, err = int32Value("DATABASE_MIN_CONNS"); err != nil {
		return Config{}, err
	}
	if err := loadPositionConfig(&cfg); err != nil {
		return Config{}, err
	}

	if cfg.DatabaseMaxConns < 1 || cfg.DatabaseMinConns < 0 || cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must be between 0 and DATABASE_MAX_CONNS")
	}

	for _, setting := range []struct {
		name   string
		target *string
	}{
		{"OTEL_SERVICE_NAME", &cfg.OTelServiceName}, {"OTEL_EXPORTER_OTLP_PROTOCOL", &cfg.OTelProtocol}, {"SERVICE_VERSION", &cfg.ServiceVersion},
	} {
		if *setting.target, err = required(setting.name); err != nil {
			return Config{}, err
		}
	}
	if _, err = required("OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
		return Config{}, err
	}
	if _, err = required("OTEL_TRACES_SAMPLER"); err != nil {
		return Config{}, err
	}
	if cfg.OTelProtocol != "grpc" && cfg.OTelProtocol != "http/protobuf" {
		return Config{}, fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL must be grpc or http/protobuf")
	}
	return cfg, nil
}

func loadPositionConfig(cfg *Config) error {
	var err error
	if cfg.PositionCheckInterval, err = durationDefault("POSITION_CHECK_INTERVAL", 30*time.Second); err != nil {
		return err
	}
	if cfg.PositionStaleAfter, err = durationDefault("POSITION_STALE_AFTER", time.Minute); err != nil {
		return err
	}
	if cfg.PushRequestTimeout, err = durationDefault("PUSH_REQUEST_TIMEOUT", time.Second); err != nil {
		return err
	}
	if cfg.PositionWorkers, err = intDefault("POSITION_WORKERS", 4); err != nil || cfg.PositionWorkers < 1 {
		return fmt.Errorf("POSITION_WORKERS must be positive")
	}
	if cfg.PositionQueueSize, err = intDefault("POSITION_QUEUE_SIZE", 100); err != nil || cfg.PositionQueueSize < 1 {
		return fmt.Errorf("POSITION_QUEUE_SIZE must be positive")
	}
	if cfg.PositionScanBatchSize, err = intDefault("POSITION_SCAN_BATCH_SIZE", 100); err != nil || cfg.PositionScanBatchSize < 1 {
		return fmt.Errorf("POSITION_SCAN_BATCH_SIZE must be positive")
	}
	cfg.PushHTTPURL = os.Getenv("PUSH_HTTP_URL")
	return nil
}

func durationDefault(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return d, nil
}

func intDefault(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return n, nil
}

func required(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", name)
	}
	return value, nil
}

func duration(name string) (time.Duration, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return d, nil
}

func int32Value(name string) (int32, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return int32(n), nil
}
