package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// NewSalt returns a random per-install salt.
func NewSalt() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Fingerprint is sha256(machine ID + per-install salt) as 64 hex characters:
// stable across restarts, different for two installs on one machine, and it
// does not reveal the machine ID.
func Fingerprint(machineID, salt string) string {
	sum := sha256.Sum256([]byte(machineID + ":" + salt))
	return hex.EncodeToString(sum[:])
}

// MachineID reads the OS machine ID (systemd's /etc/machine-id on Linux,
// the hardware UUID on macOS); empty if unavailable, in which case the salt
// alone identifies the install.
func MachineID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			return strings.TrimSpace(string(b))
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformUUID") {
					if parts := strings.Split(line, "\""); len(parts) >= 4 {
						return parts[3]
					}
				}
			}
		}
	}
	return ""
}

// MemoryMB reports total RAM in MB (0 if unknown).
func MemoryMB() uint32 {
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
				kb, _ := strconv.ParseUint(f[1], 10, 64)
				return uint32(kb / 1024)
			}
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			b, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
			return uint32(b / (1024 * 1024))
		}
	}
	return 0
}
