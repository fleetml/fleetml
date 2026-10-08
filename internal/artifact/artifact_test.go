package artifact

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vector is one case of testdata/vectors.json, which the Android and iOS SDK
// tests check too, so all clients agree on what verifies.
type vector struct {
	Name      string `json:"name"`
	Keys      []Key  `json:"keys"`
	KeyID     string `json:"key_id"`
	Signature []byte `json:"signature"`
	ModelID   string `json:"model_id"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	Now       int64  `json:"now"`
	Valid     bool   `json:"valid"`
}

func makeVectors() []vector {
	seed := func(b byte) ed25519.PrivateKey {
		s := make([]byte, ed25519.SeedSize)
		for i := range s {
			s[i] = b
		}
		return ed25519.NewKeyFromSeed(s)
	}
	current, retiring := seed(1), seed(2)
	pub := func(k ed25519.PrivateKey) ed25519.PublicKey { return k.Public().(ed25519.PublicKey) }
	cur := Key{KeyID: KeyID(pub(current)), PublicKey: pub(current)}
	old := Key{KeyID: KeyID(pub(retiring)), PublicKey: pub(retiring), NotAfter: 1_800_000_000}
	keys := []Key{cur, old}
	const model, version = "7d6f8a52-3c1e-4b0a-9f2d-1a2b3c4d5e6f", "2.1.0"
	const sha = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	sign := func(k ed25519.PrivateKey) []byte { return ed25519.Sign(k, Message(model, version, sha)) }
	good := sign(current)
	flipped := append([]byte(nil), good...)
	flipped[10] ^= 0x01
	const now = 1_790_000_000
	v := func(name string, keys []Key, keyID string, sig []byte, sha string, now int64, valid bool) vector {
		return vector{name, keys, keyID, sig, model, version, sha, now, valid}
	}
	return []vector{
		v("valid", keys, cur.KeyID, good, sha, now, true),
		v("uppercase sha is the same artifact", keys, cur.KeyID, good, "9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08", now, true),
		v("tampered signature", keys, cur.KeyID, flipped, sha, now, false),
		v("tampered sha256", keys, cur.KeyID, good, "0000000000000000000000000000000000000000000000000000000000000000", now, false),
		v("signature by another key", keys, cur.KeyID, sign(retiring), sha, now, false),
		v("unknown key", keys, "0123456789abcdef", good, sha, now, false),
		v("rotated key within its grace period", keys, old.KeyID, sign(retiring), sha, old.NotAfter-1, true),
		v("rotated key after its grace period", keys, old.KeyID, sign(retiring), sha, old.NotAfter, false),
		v("unsigned while keys are known", keys, "", nil, sha, now, false),
		v("unsigned and no keys (server without signing)", nil, "", nil, sha, now, true),
		v("signed but no keys", nil, cur.KeyID, good, sha, now, false),
	}
}

func TestVectors(t *testing.T) {
	path := "testdata/vectors.json"
	if os.Getenv("UPDATE_VECTORS") == "1" {
		b, err := json.MarshalIndent(makeVectors(), "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(b, '\n'), 0o644))
	}
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var vs []vector
	require.NoError(t, json.Unmarshal(b, &vs))
	require.Equal(t, makeVectors(), vs, "testdata/vectors.json is up to date (UPDATE_VECTORS=1 regenerates it)")
	for _, c := range vs {
		err := Verify(c.Keys, c.KeyID, c.Signature, c.ModelID, c.Version, c.SHA256, time.Unix(c.Now, 0))
		assert.Equal(t, c.Valid, err == nil, "%s: %v", c.Name, err)
	}
}

func TestMessageAndKeyID(t *testing.T) {
	assert.Equal(t, "fleetml-artifact-v1\nm\n1.0.0\nabcd", string(Message("m", "1.0.0", "ABCD")))
	pub := make([]byte, ed25519.PublicKeySize)
	assert.Len(t, KeyID(pub), 16)
}
