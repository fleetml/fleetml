// Package auth holds the credentials FleetML issues: admin API keys for the
// REST API and per-project enrollment keys for device registration.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidKey means the presented key is unknown or revoked.
	ErrInvalidKey = errors.New("invalid or revoked key")
	// ErrLastKey prevents revoking the only remaining active admin key.
	ErrLastKey = errors.New("cannot revoke the last active admin key")
)

// keyBytes is the amount of randomness in every generated key.
const keyBytes = 32

// GenerateKey returns a new random key (32 bytes, base64url without padding)
// and the hash to store for it. The plaintext is shown to the caller once and
// never stored.
func GenerateKey() (key, hash string, err error) {
	b := make([]byte, keyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating key: %w", err)
	}
	key = base64.RawURLEncoding.EncodeToString(b)
	return key, HashKey(key), nil
}

// HashKey returns the hex sha256 of a key. Keys carry 256 bits of randomness,
// so a fast hash is sufficient; there is nothing to brute-force.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// HashEqual compares two hex hashes in constant time.
func HashEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
