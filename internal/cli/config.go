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
	cfg := DefaultConfig()
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = yaml.Unmarshal(data, cfg)
	}
	if v := os.Getenv("FLEETML_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("FLEETML_OUTPUT"); v != "" {
		cfg.Output = v
	}
	return cfg
}

// SaveConfig writes config to file.
func SaveConfig(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o644)
}

// PrintConfig displays the current config.
func PrintConfig(cfg *Config) {
	fmt.Printf("server:  %s\n", cfg.Server)
	fmt.Printf("output:  %s\n", cfg.Output)
	if cfg.Project != "" {
		fmt.Printf("project: %s\n", cfg.Project)
	}
}
