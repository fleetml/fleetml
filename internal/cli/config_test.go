package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
