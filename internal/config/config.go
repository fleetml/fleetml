package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
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
	// S3PublicEndpoint is the host:port clients (devices, the CLI) reach the
	// object store at, if it differs from S3_ENDPOINT; presigned upload and
	// download URLs are signed for it.
	S3PublicEndpoint string `envconfig:"S3_PUBLIC_ENDPOINT"`
	NatsURL          string `envconfig:"NATS_URL" default:"nats://localhost:14222"`
	GRPCPort         int    `envconfig:"GRPC_PORT" default:"19090"`
	// GRPCReflection registers gRPC server reflection (for grpcurl); off by default.
	GRPCReflection bool `envconfig:"GRPC_REFLECTION" default:"false"`
	RESTPort       int  `envconfig:"REST_PORT" default:"18080"`
	// JWTSecret signs device tokens; at least MinJWTSecretLength characters.
	JWTSecret string `envconfig:"JWT_SECRET" required:"true"`
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

// MinJWTSecretLength is the shortest JWT_SECRET the server accepts.
const MinJWTSecretLength = 32

// Load reads the configuration from the environment. Errors name the
// variable; every invalid variable is reported at once.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c Config) validate() error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if c.GRPCPort < 1 || c.GRPCPort > 65535 {
		fail("GRPC_PORT must be between 1 and 65535, got %d", c.GRPCPort)
	}
	if c.RESTPort < 1 || c.RESTPort > 65535 {
		fail("REST_PORT must be between 1 and 65535, got %d", c.RESTPort)
	}
	if c.GRPCPort == c.RESTPort {
		fail("GRPC_PORT and REST_PORT must differ, both are %d", c.GRPCPort)
	}
	if len(c.JWTSecret) < MinJWTSecretLength {
		fail("JWT_SECRET must be at least %d characters, got %d (generate one with: openssl rand -base64 32)", MinJWTSecretLength, len(c.JWTSecret))
	}
	for name, v := range map[string]string{"S3_ENDPOINT": c.S3Endpoint, "S3_PUBLIC_ENDPOINT": c.S3PublicEndpoint} {
		if strings.Contains(v, "://") || strings.Contains(v, "/") {
			fail("%s must be host:port without a scheme or path (use S3_USE_SSL=true for HTTPS), got %q", name, v)
		}
	}
	if c.S3Endpoint == "" {
		fail("S3_ENDPOINT must not be empty")
	}
	for name, d := range map[string]time.Duration{
		"DEPLOY_TICK_INTERVAL": c.DeployTickInterval, "EXPERIMENT_TICK_INTERVAL": c.ExperimentTickInterval, "DRIFT_TICK_INTERVAL": c.DriftTickInterval,
	} {
		if d < time.Second {
			fail("%s must be at least 1s, got %s", name, d)
		}
	}
	if c.DeviceTokenTTL < 24*time.Hour {
		fail("DEVICE_TOKEN_TTL must be at least 24h, got %s", c.DeviceTokenTTL)
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "warning", "error":
	default:
		fail("LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel)
	}
	switch strings.ToLower(c.LogFormat) {
	case "json", "text":
	default:
		fail("LOG_FORMAT must be json or text, got %q", c.LogFormat)
	}
	if c.AlertWebhookURL != "" {
		if u, err := url.Parse(c.AlertWebhookURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fail("ALERT_WEBHOOK_URL must be an http(s) URL, got %q", c.AlertWebhookURL)
		}
	}
	return errors.Join(errs...)
}
