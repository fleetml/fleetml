package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds CLI configuration.
type Config struct {
	Server  string `yaml:"server"`
	Output  string `yaml:"output"`
	Project string `yaml:"project"`
	// APIKey is the admin API key sent as "Authorization: Bearer".
	APIKey string `yaml:"api_key,omitempty"`
}

// DefaultConfig returns the default CLI configuration.
func DefaultConfig() *Config {
	return &Config{Server: "http://localhost:18080", Output: "table"}
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleetml")
}

func configPath() string {
	return filepath.Join(configDir(), "config.yaml")
}

// LoadConfig loads config from file, then overlays env vars.
func LoadConfig() *Config {
	cfg := LoadFileConfig()
	if v := os.Getenv("FLEETML_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("FLEETML_OUTPUT"); v != "" {
		cfg.Output = v
	}
	if v := os.Getenv("FLEETML_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	return cfg
}

// LoadFileConfig loads config from file only, without env overrides.
func LoadFileConfig() *Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = yaml.Unmarshal(data, cfg)
	}
	return cfg
}

// SaveConfig writes config to file.
func SaveConfig(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	// The file may hold an API key: keep it private to the user.
	if err := os.WriteFile(configPath(), data, 0o600); err != nil {
		return err
	}
	return os.Chmod(configPath(), 0o600)
}

// PrintConfig displays the current config.
func PrintConfig(cfg *Config) {
	fmt.Printf("server:  %s\n", cfg.Server)
	fmt.Printf("output:  %s\n", cfg.Output)
	if cfg.Project != "" {
		fmt.Printf("project: %s\n", cfg.Project)
	}
	fmt.Printf("api_key: %s\n", maskKey(cfg.APIKey))
}

// maskKey shows only enough of a key to tell keys apart.
func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}
