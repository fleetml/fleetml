package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "http://localhost:18080", cfg.Server)
	assert.Equal(t, "table", cfg.Output)
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	t.Setenv("FLEETML_SERVER", "http://custom:9090")
	t.Setenv("FLEETML_OUTPUT", "json")
	cfg := LoadConfig()
	assert.Equal(t, "http://custom:9090", cfg.Server)
	assert.Equal(t, "json", cfg.Output)
}

func TestLoadConfig_NoEnv(t *testing.T) {
	t.Setenv("FLEETML_SERVER", "")
	t.Setenv("FLEETML_OUTPUT", "")
	cfg := LoadConfig()
	assert.NotEmpty(t, cfg.Server)
	assert.NotEmpty(t, cfg.Output)
}

func TestSaveConfig_PrivateFileAndAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLEETML_API_KEY", "")
	cfg := DefaultConfig()
	cfg.APIKey = "secret-key-value"
	require.NoError(t, SaveConfig(cfg))

	info, err := os.Stat(configPath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Equal(t, "secret-key-value", LoadConfig().APIKey)

	t.Setenv("FLEETML_API_KEY", "from-env")
	assert.Equal(t, "from-env", LoadConfig().APIKey)
	assert.Equal(t, "secr…alue", maskKey("secret-key-value"))
}
