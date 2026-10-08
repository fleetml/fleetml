package config

import (
	"fmt"

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
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat   string `envconfig:"LOG_FORMAT" default:"json"`
	CORSOrigins string `envconfig:"CORS_ORIGINS" default:"http://localhost:5173,http://localhost:18080"`
	// AdminKey is stored as the "bootstrap" admin API key if no key exists yet.
	AdminKey string `envconfig:"FLEETML_ADMIN_KEY"`
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
	if c.GRPCPort == c.RESTPort {
		return fmt.Errorf("GRPC_PORT and REST_PORT must differ, both are %d", c.GRPCPort)
	}
	return nil
}
