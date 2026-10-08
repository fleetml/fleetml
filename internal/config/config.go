package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	S3Endpoint  string `envconfig:"S3_ENDPOINT" default:"localhost:19000"`
	S3Bucket    string `envconfig:"S3_BUCKET" default:"fleetml-models"`
	S3AccessKey string `envconfig:"S3_ACCESS_KEY" required:"true"`
	S3SecretKey string `envconfig:"S3_SECRET_KEY" required:"true"`
	S3UseSSL    bool   `envconfig:"S3_USE_SSL" default:"false"`
	NatsURL     string `envconfig:"NATS_URL" default:"nats://localhost:14222"`
	RedisURL    string `envconfig:"REDIS_URL" default:"redis://localhost:16379"`
	GRPCPort    int    `envconfig:"GRPC_PORT" default:"19090"`
	RESTPort    int    `envconfig:"REST_PORT" default:"18080"`
	JWTSecret   string `envconfig:"JWT_SECRET" required:"true"`
	// DeviceTokenTTL is the lifetime of device tokens (refreshable in their last 7 days).
	DeviceTokenTTL time.Duration `envconfig:"DEVICE_TOKEN_TTL" default:"720h"`
	LogLevel       string        `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat      string        `envconfig:"LOG_FORMAT" default:"json"`
	CORSOrigins    string        `envconfig:"CORS_ORIGINS" default:"http://localhost:5173,http://localhost:18080"`
	// AdminKey is stored as the "bootstrap" admin API key if no key exists yet.
	AdminKey string `envconfig:"FLEETML_ADMIN_KEY"`
	// AlertWebhookURL receives rollback and drift alerts as signed JSON POSTs (optional).
	AlertWebhookURL string `envconfig:"ALERT_WEBHOOK_URL"`
	// AlertWebhookSecret signs webhook bodies: X-FleetML-Signature: sha256=<HMAC-SHA256>.
	AlertWebhookSecret string `envconfig:"ALERT_WEBHOOK_SECRET"`
	// Background loop intervals. Short values are for tests and demos.
	DeployTickInterval     time.Duration `envconfig:"DEPLOY_TICK_INTERVAL" default:"30s"`
	ExperimentTickInterval time.Duration `envconfig:"EXPERIMENT_TICK_INTERVAL" default:"60s"`
	DriftTickInterval      time.Duration `envconfig:"DRIFT_TICK_INTERVAL" default:"5m"`
	// MigrateOnStart applies embedded DB migrations before the server starts.
	MigrateOnStart bool `envconfig:"MIGRATE_ON_START" default:"false"`
}

func (c Config) GRPCAddr() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

func (c Config) RESTAddr() string {
	return fmt.Sprintf(":%d", c.RESTPort)
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}

func (c Config) validate() error {
	if c.GRPCPort < 1 || c.GRPCPort > 65535 {
		return fmt.Errorf("GRPC_PORT must be between 1 and 65535, got %d", c.GRPCPort)
	}
	if c.RESTPort < 1 || c.RESTPort > 65535 {
		return fmt.Errorf("REST_PORT must be between 1 and 65535, got %d", c.RESTPort)
	}
	for name, d := range map[string]time.Duration{
		"DEPLOY_TICK_INTERVAL": c.DeployTickInterval, "EXPERIMENT_TICK_INTERVAL": c.ExperimentTickInterval, "DRIFT_TICK_INTERVAL": c.DriftTickInterval,
	} {
		if d < time.Second {
			return fmt.Errorf("%s must be at least 1s, got %s", name, d)
		}
	}
	if c.DeviceTokenTTL < 24*time.Hour {
		return fmt.Errorf("DEVICE_TOKEN_TTL must be at least 24h, got %s", c.DeviceTokenTTL)
	}
	if c.GRPCPort == c.RESTPort {
		return fmt.Errorf("GRPC_PORT and REST_PORT must differ, both are %d", c.GRPCPort)
	}
	return nil
}
