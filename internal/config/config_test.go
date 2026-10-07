package config

import (
	"os"
	"testing"

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
		"JWT_SECRET":    "test-secret",
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
	assert.Equal(t, "redis://localhost:16379", cfg.RedisURL)
	assert.Equal(t, 19090, cfg.GRPCPort)
	assert.Equal(t, 18080, cfg.RESTPort)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.False(t, cfg.MigrateOnStart)
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Clearenv()
	_, err := Load()
	assert.Error(t, err)
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
