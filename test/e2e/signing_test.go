//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/artifact"
)

func keysOf(in []*fleetmlv1.SigningKey) []artifact.Key {
	out := make([]artifact.Key, 0, len(in))
	for _, k := range in {
		out = append(out, artifact.Key{KeyID: k.KeyId, PublicKey: k.PublicKey, NotAfter: k.NotAfter})
	}
	return out
}

func verifyAssignment(keys []artifact.Key, a *fleetmlv1.ModelAssignment) error {
	return artifact.Verify(keys, a.SigningKeyId, a.Signature, a.ModelId, a.Version, a.BlobSha256, time.Now())
}

// V3.1: published variants are signed; devices get the keys and signed
// assignments; a rotation re-signs and keeps the old key for its grace.
func TestE2E_ArtifactSigning(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v1, _, _ := e.newVersion(t, f, "1.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": labels})

	var key struct {
		Active struct {
			KeyID string `json:"key_id"`
		} `json:"active"`
		Keys     []map[string]any `json:"keys"`
		Resigned *int             `json:"resigned_variants"`
	}
	require.Equal(t, 200, e.call(t, "GET", "/api/v1/projects/"+f.project+"/signing-key", nil, &key))
	first := key.Active.KeyID
	require.NotEmpty(t, first, "publishing created the project's key")

	reg, err := e.devices.Register(t.Context(), &fleetmlv1.RegisterRequest{
		Fingerprint: randHex(32), Os: "ANDROID", RamMb: 4096, EnrollmentKey: f.enrollKey, Labels: labels,
	})
	require.NoError(t, err)
	require.Len(t, reg.SigningKeys, 1)
	assert.Equal(t, first, reg.SigningKeys[0].KeyId)
	d := &device{id: reg.DeviceId, token: reg.AuthToken, loaded: map[string]*fleetmlv1.ModelAssignment{}}
	resp := e.heartbeat(t, d)
	require.Len(t, resp.Assignments, 1)
	a := resp.Assignments[0]
	assert.Equal(t, first, a.SigningKeyId)
	require.NoError(t, verifyAssignment(keysOf(resp.SigningKeys), a))
	zero := "0000000000000000000000000000000000000000000000000000000000000000"
	assert.Error(t, artifact.Verify(keysOf(resp.SigningKeys), a.SigningKeyId, a.Signature, a.ModelId, a.Version, zero, time.Now()),
		"the signature covers the sha256")

	assert.Equal(t, 400, e.call(t, "POST", "/api/v1/projects/"+f.project+"/signing-key/rotate", map[string]string{"grace": "forever"}, nil))
	assert.Equal(t, 404, e.call(t, "POST", "/api/v1/projects/00000000-0000-0000-0000-000000000001/signing-key/rotate", nil, nil))
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/projects/"+f.project+"/signing-key/rotate", map[string]string{"grace": "1h"}, &key))
	require.NotNil(t, key.Resigned)
	assert.Equal(t, 1, *key.Resigned)
	assert.NotEqual(t, first, key.Active.KeyID)
	require.Len(t, key.Keys, 2)

	// The device now gets both keys; the old key carries the end of its grace.
	resp = e.heartbeat(t, d)
	keys := keysOf(resp.SigningKeys)
	require.Len(t, keys, 2)
	assert.Equal(t, key.Active.KeyID, keys[0].KeyID)
	assert.InDelta(t, time.Now().Add(time.Hour).Unix(), keys[1].NotAfter, 60)
	require.NoError(t, verifyAssignment(keys, a), "an assignment signed before the rotation verifies during the grace")
	assert.Error(t, artifact.Verify(keys, a.SigningKeyId, a.Signature, a.ModelId, a.Version, a.BlobSha256, time.Now().Add(2*time.Hour)), "and not after it")

	// A device that registers now gets the variant signed with the new key.
	d2 := e.register(t, f.enrollKey, labels)
	resp = e.heartbeat(t, d2)
	require.Len(t, resp.Assignments, 1)
	assert.Equal(t, key.Active.KeyID, resp.Assignments[0].SigningKeyId)
	require.NoError(t, verifyAssignment(keysOf(resp.SigningKeys), resp.Assignments[0]))
}
