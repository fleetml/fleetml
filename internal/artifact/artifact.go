// Package artifact defines what a project's signing key signs for a model
// artifact and how a device checks it. It has no dependencies beyond the
// standard library, so the device agent can use it.
package artifact

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Message is the byte string signed for an artifact: model ID, version and
// the file's sha256 (lowercase hex).
func Message(modelID, version, sha256Hex string) []byte {
	return []byte("fleetml-artifact-v1\n" + modelID + "\n" + version + "\n" + strings.ToLower(sha256Hex))
}

// KeyID identifies a public key: the first 16 hex digits of its sha256.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// Key is a public key a device accepts.
type Key struct {
	KeyID     string `json:"key_id"`
	PublicKey []byte `json:"public_key"`
	// NotAfter is the end of a rotated-out key's grace period (Unix
	// seconds); 0 for the current key.
	NotAfter int64 `json:"not_after,omitempty"`
}

// ErrNoKeys is returned for a signed artifact when the device has no keys.
var ErrNoKeys = errors.New("the artifact is signed but no signing keys are known")

// Verify checks an artifact's signature with the device's keys. With no keys
// (a server without artifact signing) unsigned artifacts are accepted.
func Verify(keys []Key, keyID string, signature []byte, modelID, version, sha256Hex string, now time.Time) error {
	if len(keys) == 0 {
		if len(signature) == 0 {
			return nil
		}
		return ErrNoKeys
	}
	if len(signature) == 0 {
		return errors.New("the artifact is not signed")
	}
	for _, k := range keys {
		if k.KeyID != keyID {
			continue
		}
		if k.NotAfter > 0 && now.Unix() >= k.NotAfter {
			return fmt.Errorf("signing key %s was rotated out and its grace period has ended", keyID)
		}
		if len(k.PublicKey) != ed25519.PublicKeySize || !ed25519.Verify(k.PublicKey, Message(modelID, version, sha256Hex), signature) {
			return fmt.Errorf("the signature does not verify with key %s", keyID)
		}
		return nil
	}
	return fmt.Errorf("unknown signing key %q", keyID)
}
