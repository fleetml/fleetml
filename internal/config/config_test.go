package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setEnv(t *testing.T, kvs map[string]string) {
	t.Helper()
	for k, v := range kvs {
		t.Setenv(k, v)
	}
}

func minimalEnv(t *testing.T) {
	t.Helper()
	setEnv(t, map[string]string{
		"DATABASE_URL":  "postgres://localhost/test",
		"S3_ACCESS_KEY": "key",
		"S3_SECRET_KEY": "secret",
		"JWT_SECRET":    "test-secret-at-least-32-characters",
	})
}

func TestLoad_Defaults(t *testing.T) {
	minimalEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "postgres://localhost/test", cfg.DatabaseURL)
	assert.Equal(t, "localhost:19000", cfg.S3Endpoint)
	assert.Equal(t, "fleetml-models", cfg.S3Bucket)
	assert.Equal(t, false, cfg.S3UseSSL)
	assert.Equal(t, "nats://localhost:14222", cfg.NatsURL)
	assert.Equal(t, 19090, cfg.GRPCPort)
	assert.Equal(t, 18080, cfg.RESTPort)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.False(t, cfg.MigrateOnStart)
	assert.Equal(t, "http://localhost:5173,http://localhost:18080", cfg.CORSOrigins)
	assert.Equal(t, 30*24*time.Hour, cfg.DeviceTokenTTL)
}

func TestLoad_DeviceTokenTTL(t *testing.T) {
	minimalEnv(t)
	t.Setenv("DEVICE_TOKEN_TTL", "168h")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, cfg.DeviceTokenTTL)

	t.Setenv("DEVICE_TOKEN_TTL", "1h")
	_, err = Load()
	assert.ErrorContains(t, err, "DEVICE_TOKEN_TTL")
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Clearenv()
	_, err := Load()
	assert.ErrorContains(t, err, "DATABASE_URL", "the error names the missing variable")
}

// Every invalid variable is reported, each by name.
func TestLoad_Invalid(t *testing.T) {
	minimalEnv(t)
	setEnv(t, map[string]string{
		"JWT_SECRET":                           "short",
		"S3_ENDPOINT":                          "http://localhost:19000",
		"S3_PUBLIC_ENDPOINT":                   "10.0.2.2:19000/bucket",
		"LOG_LEVEL":                            "verbose",
		"LOG_FORMAT":                           "xml",
		"ALERT_WEBHOOK_URL":                    "hooks.example.com/x",
		"DEVICE_BACKGROUND_HEARTBEAT_INTERVAL": "5m",
		"DEVICE_DOWNLOAD_POLICY":               "wifi",
		"DASHBOARD_URL":                        "fleet.example.com",
		"SMTP_HOST":                            "smtp.example.com",
	})
	_, err := Load()
	require.Error(t, err)
	for _, name := range []string{"JWT_SECRET must be at least 32", "S3_ENDPOINT must be host:port", "S3_PUBLIC_ENDPOINT must be host:port",
		"LOG_LEVEL", "LOG_FORMAT", "ALERT_WEBHOOK_URL", "DEVICE_BACKGROUND_HEARTBEAT_INTERVAL", "DEVICE_DOWNLOAD_POLICY",
		"DASHBOARD_URL", "SMTP_HOST and SMTP_FROM"} {
		assert.ErrorContains(t, err, name)
	}
}

func TestLoad_CustomPorts(t *testing.T) {
	minimalEnv(t)
	t.Setenv("GRPC_PORT", "50051")
	t.Setenv("REST_PORT", "3000")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 50051, cfg.GRPCPort)
	assert.Equal(t, 3000, cfg.RESTPort)
}

func TestLoad_SamePortsFails(t *testing.T) {
	minimalEnv(t)
	t.Setenv("GRPC_PORT", "8080")
	t.Setenv("REST_PORT", "8080")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must differ")
}

func TestLoad_InvalidPortRange(t *testing.T) {
	minimalEnv(t)
	t.Setenv("GRPC_PORT", "0")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GRPC_PORT")
}

func TestConfig_Addrs(t *testing.T) {
	cfg := Config{GRPCPort: 9090, RESTPort: 8080}
	assert.Equal(t, ":9090", cfg.GRPCAddr())
	assert.Equal(t, ":8080", cfg.RESTAddr())
}
