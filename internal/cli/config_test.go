package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	tempHome(t)
	t.Setenv("FLEETML_SERVER", "http://custom:9090")
	t.Setenv("FLEETML_OUTPUT", "json")
	cfg := LoadConfig()
	assert.Equal(t, "http://custom:9090", cfg.Server)
	assert.Equal(t, "json", cfg.Output)
}

func TestLoadConfig_NoEnv(t *testing.T) {
	tempHome(t)
	t.Setenv("FLEETML_SERVER", "")
	t.Setenv("FLEETML_OUTPUT", "")
	cfg := LoadConfig()
	assert.NotEmpty(t, cfg.Server)
	assert.NotEmpty(t, cfg.Output)
}

// tempHome points the user's home directory at a temporary directory:
// os.UserHomeDir reads HOME on Unix and USERPROFILE on Windows.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestConfigPath_UnderHome(t *testing.T) {
	home := tempHome(t)
	assert.Equal(t, filepath.Join(home, ".fleetml", "config.yaml"), configPath())
}

func TestSaveConfig_PrivateFileAndAPIKey(t *testing.T) {
	tempHome(t)
	t.Setenv("FLEETML_API_KEY", "")
	cfg := DefaultConfig()
	cfg.APIKey = "secret-key-value"
	require.NoError(t, SaveConfig(cfg))

	info, err := os.Stat(configPath())
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Windows has no Unix permission bits: chmod only sets the read-only
		// flag, and 0600 leaves the file writable. It must not fail.
		assert.Equal(t, os.FileMode(0o666), info.Mode().Perm())
	} else {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	assert.Equal(t, "secret-key-value", LoadConfig().APIKey)

	t.Setenv("FLEETML_API_KEY", "from-env")
	assert.Equal(t, "from-env", LoadConfig().APIKey)
	assert.Equal(t, "secr…alue", maskKey("secret-key-value"))
}

func TestSaveConfig_Twice(t *testing.T) {
	tempHome(t)
	require.NoError(t, SaveConfig(DefaultConfig()))
	cfg := DefaultConfig()
	cfg.Project = "p1"
	require.NoError(t, SaveConfig(cfg), "rewriting an existing config file")
	assert.Equal(t, "p1", LoadFileConfig().Project)
}

// A config file edited on Windows may have CRLF line endings and a UTF-8 BOM.
func TestLoadFileConfig_WindowsLineEndings(t *testing.T) {
	tempHome(t)
	require.NoError(t, os.MkdirAll(configDir(), 0o700))
	data := "\ufeffserver: http://fleet.example:18080\r\noutput: json\r\napi_key: k-123\r\n"
	require.NoError(t, os.WriteFile(configPath(), []byte(data), 0o600))
	cfg := LoadFileConfig()
	assert.Equal(t, "http://fleet.example:18080", cfg.Server)
	assert.Equal(t, "json", cfg.Output)
	assert.Equal(t, "k-123", cfg.APIKey)
}

// set-key without the argument reads the key from standard input (a pipe
// here; a hidden prompt at a terminal), trimmed; an empty one is refused.
func TestSetKeyFromStdin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	run := func(in string, args ...string) error {
		root := NewRootCmd()
		root.SetIn(strings.NewReader(in))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(append([]string{"config", "set-key"}, args...))
		return root.Execute()
	}
	require.NoError(t, run("key-from-stdin\n"))
	assert.Equal(t, "key-from-stdin", LoadFileConfig().APIKey)
	require.NoError(t, run("", "key-from-arg"))
	assert.Equal(t, "key-from-arg", LoadFileConfig().APIKey)
	err := run("\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API key")
	assert.Equal(t, "key-from-arg", LoadFileConfig().APIKey)
}
