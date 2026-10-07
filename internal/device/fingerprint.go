package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ValidateFingerprint checks if a fingerprint is a valid SHA256 hex string (64 chars).
func ValidateFingerprint(fp string) error {
	if !fingerprintPattern.MatchString(fp) {
		return fmt.Errorf("fingerprint must be a 64-character lowercase hex string")
	}
	return nil
}

// GenerateFingerprint creates a fingerprint from device hardware attributes.
// Used by tests; real fingerprints come from the device agent.
func GenerateFingerprint(os, chipset string, ramMB int) string {
	data := fmt.Sprintf("%s:%s:%d", os, chipset, ramMB)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
