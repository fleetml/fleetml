package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
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
	// S3PublicUseSSL is whether those URLs are https; unset, it is S3_USE_SSL.
	// They differ when TLS is terminated in front of the store (an Ingress
	// before the bundled SeaweedFS) or only on the internal connection.
	S3PublicUseSSL string `envconfig:"S3_PUBLIC_USE_SSL"`
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:14222"`
	GRPCPort       int    `envconfig:"GRPC_PORT" default:"19090"`
	// GRPCReflection registers gRPC server reflection (for grpcurl); off by default.
	GRPCReflection bool `envconfig:"GRPC_REFLECTION" default:"false"`
	RESTPort       int  `envconfig:"REST_PORT" default:"18080"`
	// JWTSecret signs device tokens; at least MinJWTSecretLength characters.
	JWTSecret string `envconfig:"JWT_SECRET" required:"true"`
	// SigningMasterKey encrypts the projects' artifact signing keys; at least
	// MinJWTSecretLength characters. Changing it makes the stored keys unusable.
	SigningMasterKey string `envconfig:"SIGNING_MASTER_KEY" required:"true"`
	// DeviceTokenTTL is the lifetime of device tokens (refreshable in their last 7 days).
	DeviceTokenTTL time.Duration `envconfig:"DEVICE_TOKEN_TTL" default:"720h"`
	LogLevel       string        `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat      string        `envconfig:"LOG_FORMAT" default:"json"`
	CORSOrigins    string        `envconfig:"CORS_ORIGINS" default:"http://localhost:5173,http://localhost:18080"`
	// SessionTTL is how long a dashboard login lasts.
	SessionTTL time.Duration `envconfig:"SESSION_TTL" default:"12h"`
	// AdminKey is stored as the "bootstrap" admin API key if no key exists yet.
	AdminKey string `envconfig:"FLEETML_ADMIN_KEY"`
	// AlertWebhookURL receives rollback and drift alerts as signed JSON POSTs (optional).
	AlertWebhookURL string `envconfig:"ALERT_WEBHOOK_URL"`
	// AlertWebhookSecret signs webhook bodies: X-FleetML-Signature: sha256=<HMAC-SHA256>.
	AlertWebhookSecret string `envconfig:"ALERT_WEBHOOK_SECRET"`
	// DashboardURL is where Slack and email alerts link to (optional).
	DashboardURL string `envconfig:"DASHBOARD_URL"`
	// SMTP server for email alert sinks; email sinks need SMTP_HOST and SMTP_FROM.
	SMTPHost     string `envconfig:"SMTP_HOST"`
	SMTPPort     int    `envconfig:"SMTP_PORT" default:"587"`
	SMTPUsername string `envconfig:"SMTP_USERNAME"`
	SMTPPassword string `envconfig:"SMTP_PASSWORD"`
	SMTPFrom     string `envconfig:"SMTP_FROM"`
	// Background loop intervals. Short values are for tests and demos.
	DeployTickInterval     time.Duration `envconfig:"DEPLOY_TICK_INTERVAL" default:"30s"`
	ExperimentTickInterval time.Duration `envconfig:"EXPERIMENT_TICK_INTERVAL" default:"60s"`
	DriftTickInterval      time.Duration `envconfig:"DRIFT_TICK_INTERVAL" default:"5m"`
	// Device settings sent at registration and with every heartbeat.
	DeviceHeartbeatInterval           time.Duration `envconfig:"DEVICE_HEARTBEAT_INTERVAL" default:"60s"`
	DeviceBackgroundHeartbeatInterval time.Duration `envconfig:"DEVICE_BACKGROUND_HEARTBEAT_INTERVAL" default:"1h"`
	DeviceTelemetryBatchSize          int           `envconfig:"DEVICE_TELEMETRY_BATCH_SIZE" default:"100"`
	DeviceTelemetryFlushInterval      time.Duration `envconfig:"DEVICE_TELEMETRY_FLUSH_INTERVAL" default:"30s"`
	DeviceDownloadPolicy              string        `envconfig:"DEVICE_DOWNLOAD_POLICY" default:"unmetered_or_charging"`
	// DeltaMaxSizeMB: models up to this size get a delta from their previous
	// version (diffing needs about ten times the model size in memory); 0 disables.
	DeltaMaxSizeMB int `envconfig:"DELTA_MAX_SIZE_MB" default:"128"`
	// MigrateOnStart applies embedded DB migrations before the server starts.
	MigrateOnStart bool `envconfig:"MIGRATE_ON_START" default:"false"`
}

// PublicUseSSL is whether presigned URLs are https: S3_PUBLIC_USE_SSL, or
// S3_USE_SSL when it is unset.
func (c Config) PublicUseSSL() bool {
	if v, err := strconv.ParseBool(c.S3PublicUseSSL); err == nil {
		return v
	}
	return c.S3UseSSL
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
	if len(c.SigningMasterKey) < MinJWTSecretLength {
		fail("SIGNING_MASTER_KEY must be at least %d characters, got %d (generate one with: openssl rand -base64 32)", MinJWTSecretLength, len(c.SigningMasterKey))
	} else if c.SigningMasterKey == c.JWTSecret {
		fail("SIGNING_MASTER_KEY must differ from JWT_SECRET")
	}
	for name, v := range map[string]string{"S3_ENDPOINT": c.S3Endpoint, "S3_PUBLIC_ENDPOINT": c.S3PublicEndpoint} {
		if strings.Contains(v, "://") || strings.Contains(v, "/") {
			fail("%s must be host:port without a scheme or path (use S3_USE_SSL=true for HTTPS), got %q", name, v)
		}
	}
	if c.S3Endpoint == "" {
		fail("S3_ENDPOINT must not be empty")
	}
	if c.S3PublicUseSSL != "" {
		if _, err := strconv.ParseBool(c.S3PublicUseSSL); err != nil {
			fail("S3_PUBLIC_USE_SSL must be true or false, got %q", c.S3PublicUseSSL)
		}
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
	if c.DeviceHeartbeatInterval < 5*time.Second {
		fail("DEVICE_HEARTBEAT_INTERVAL must be at least 5s, got %s", c.DeviceHeartbeatInterval)
	}
	if c.DeviceBackgroundHeartbeatInterval < 15*time.Minute {
		fail("DEVICE_BACKGROUND_HEARTBEAT_INTERVAL must be at least 15m (the OS minimum), got %s", c.DeviceBackgroundHeartbeatInterval)
	}
	if c.DeviceTelemetryBatchSize < 1 || c.DeviceTelemetryBatchSize > 1000 {
		fail("DEVICE_TELEMETRY_BATCH_SIZE must be between 1 and 1000, got %d", c.DeviceTelemetryBatchSize)
	}
	if c.DeviceTelemetryFlushInterval < time.Second {
		fail("DEVICE_TELEMETRY_FLUSH_INTERVAL must be at least 1s, got %s", c.DeviceTelemetryFlushInterval)
	}
	switch c.DeviceDownloadPolicy {
	case "unmetered_or_charging", "unmetered", "any":
	default:
		fail("DEVICE_DOWNLOAD_POLICY must be unmetered_or_charging, unmetered or any, got %q", c.DeviceDownloadPolicy)
	}
	if c.AlertWebhookURL != "" {
		if u, err := url.Parse(c.AlertWebhookURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fail("ALERT_WEBHOOK_URL must be an http(s) URL, got %q", c.AlertWebhookURL)
		}
	}
	if c.DeltaMaxSizeMB < 0 {
		fail("DELTA_MAX_SIZE_MB must be 0 (no deltas) or more, got %d", c.DeltaMaxSizeMB)
	}
	if c.SessionTTL < 5*time.Minute {
		fail("SESSION_TTL must be at least 5m, got %s", c.SessionTTL)
	}
	if c.DashboardURL != "" {
		if u, err := url.Parse(c.DashboardURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fail("DASHBOARD_URL must be an http(s) URL, got %q", c.DashboardURL)
		}
	}
	if (c.SMTPHost == "") != (c.SMTPFrom == "") {
		fail("SMTP_HOST and SMTP_FROM must be set together")
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		fail("SMTP_PORT must be a port number, got %d", c.SMTPPort)
	}
	return errors.Join(errs...)
}
