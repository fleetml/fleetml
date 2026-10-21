// Package agent is the FleetML device agent for Linux machines (and macOS for
// testing): the same device protocol and rules as the Android and iOS SDKs,
// for models that a local inference process loads from disk.
package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is read from a YAML file; flags override it.
type Config struct {
	// Server is the FleetML gRPC address, host:port.
	Server string `yaml:"server"`
	TLS    bool   `yaml:"tls"`
	// EnrollmentKey registers the device into a project.
	EnrollmentKey string            `yaml:"enrollment_key"`
	Labels        map[string]string `yaml:"labels"`
	// StateDir holds the registration, the telemetry queue and partial downloads.
	StateDir string `yaml:"state_dir"`
	// ModelsDir holds <model>/<version>/model.bin and the <model>/current symlink.
	ModelsDir string `yaml:"models_dir"`
	// Validator is a shell command run against a downloaded model before it is
	// activated; {file} is replaced by the file path. Exit code 0 means valid.
	// Empty accepts every model that matches its sha256.
	Validator string `yaml:"validator"`
	// PostActivate is a shell command run after a model version becomes
	// active, e.g. to restart the inference process.
	PostActivate string `yaml:"post_activate"`
	// TelemetryListen enables the local HTTP endpoint for inference events,
	// e.g. 127.0.0.1:9601. Empty disables it.
	TelemetryListen string `yaml:"telemetry_listen"`
	// Metered marks the machine's network as metered (cellular modem);
	// OnBattery marks it as running on battery. Both feed the download policy.
	Metered   bool `yaml:"metered"`
	OnBattery bool `yaml:"on_battery"`
	// HeartbeatInterval is used until the server sends its own.
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	// DownloadHostOverride (host:port) replaces the host and port of every
	// model download URL the server hands out, for a server whose
	// S3_PUBLIC_ENDPOINT this machine cannot reach. The request still carries
	// the original Host header, which the URL's signature covers.
	DownloadHostOverride string `yaml:"download_host_override"`
}

// LoadConfig reads a YAML file (if path is not empty) over the defaults.
func LoadConfig(path string) (*Config, error) {
	c := &Config{StateDir: "/var/lib/fleetml-agent", HeartbeatInterval: 30 * time.Second}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		if err := yaml.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	return c, nil
}

// Validate fills derived defaults and checks the required fields.
func (c *Config) Validate() error {
	if c.Server == "" {
		return fmt.Errorf("server is required (host:port of the FleetML gRPC API)")
	}
	if c.StateDir == "" {
		return fmt.Errorf("state_dir is required")
	}
	if c.ModelsDir == "" {
		c.ModelsDir = filepath.Join(c.StateDir, "models")
	}
	if c.HeartbeatInterval < time.Second {
		c.HeartbeatInterval = 30 * time.Second
	}
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	if c.DownloadHostOverride != "" {
		if _, _, err := net.SplitHostPort(c.DownloadHostOverride); err != nil || strings.Contains(c.DownloadHostOverride, "/") {
			return fmt.Errorf("download_host_override must be host:port, got %q", c.DownloadHostOverride)
		}
	}
	return nil
}
