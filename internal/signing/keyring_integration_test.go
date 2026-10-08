//go:build integration

package signing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/artifact"
	"github.com/fleetml/fleetml/internal/dbtest"
)

func deviceKeys(keys []Key) []artifact.Key {
	out := make([]artifact.Key, 0, len(keys))
	for _, k := range keys {
		ak := artifact.Key{KeyID: k.KeyID, PublicKey: k.PublicKey}
		if k.NotAfter != nil {
			ak.NotAfter = k.NotAfter.Unix()
		}
		out = append(out, ak)
	}
	return out
}

func TestKeyring_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	_, err := NewKeyring(pool, "short")
	require.Error(t, err)
	kr, err := NewKeyring(pool, "integration-master-key-0123456789abcdef")
	require.NoError(t, err)

	f := dbtest.NewVersion(t, pool)
	project, model := f.ProjectID.String(), f.ModelID.String()
	_, v2 := dbtest.NewVersionOf(t, pool, f.ModelID, "2.0.0")
	dev := dbtest.NewDevice(t, pool, "")
	_, err = pool.Exec(ctx, `UPDATE devices SET project_id = $1 WHERE id = $2`, f.ProjectID, dev)
	require.NoError(t, err)

	// Variants published before signing are signed at startup, with a key created on first use.
	n, err := kr.Backfill(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 2)
	keys, err := kr.DeviceKeys(ctx, dev.String())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	first := keys[0].KeyID
	sigs, err := kr.VariantSignatures(ctx, []string{f.VariantID.String(), v2.String()})
	require.NoError(t, err)
	require.Len(t, sigs, 2)
	verify := func(keys []Key, variant, version string, now time.Time) error {
		s := sigs[variant]
		return artifact.Verify(deviceKeys(keys), s.KeyID, s.Signature, model, version, "sha", now)
	}
	require.NoError(t, verify(keys, f.VariantID.String(), "1.0.0", time.Now()))
	assert.Error(t, verify(keys, f.VariantID.String(), "2.0.0", time.Now()), "a signature is bound to its version")

	// A variant whose stored signature was tampered with is not carried over by a rotation.
	_, err = pool.Exec(ctx, `UPDATE model_variants SET signature = decode(repeat('00', 64), 'hex') WHERE id = $1`, v2)
	require.NoError(t, err)
	newKey, resigned, err := kr.Rotate(ctx, project, time.Hour)
	require.NoError(t, err)
	assert.NotEqual(t, first, newKey)
	assert.Equal(t, 1, resigned, "only the variant whose signature verified is signed again")

	keys, err = kr.Keys(ctx, project)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.Equal(t, StatusActive, keys[0].Status)
	assert.Equal(t, newKey, keys[0].KeyID)
	assert.Equal(t, StatusRetiring, keys[1].Status)
	require.NotNil(t, keys[1].NotAfter)
	assert.WithinDuration(t, time.Now().Add(time.Hour), *keys[1].NotAfter, time.Minute)

	sigs, err = kr.VariantSignatures(ctx, []string{f.VariantID.String(), v2.String()})
	require.NoError(t, err)
	assert.Equal(t, newKey, sigs[f.VariantID.String()].KeyID)
	require.NoError(t, verify(keys, f.VariantID.String(), "1.0.0", time.Now()))
	assert.Error(t, verify(keys, v2.String(), "2.0.0", time.Now()), "the tampered signature stays invalid")

	// Signing a new variant uses the new key. Rotating again with no grace
	// drops that key at once; the first key is still in its hour of grace.
	require.NoError(t, kr.SignVariant(ctx, v2.String()))
	third, _, err := kr.Rotate(ctx, project, 0)
	require.NoError(t, err)
	keys, err = kr.Keys(ctx, project)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.Equal(t, []string{third, first}, []string{keys[0].KeyID, keys[1].KeyID}, "a key whose grace has ended is no longer offered")

	// Another master key cannot use the stored keys.
	other, err := NewKeyring(pool, "a-different-master-key-0123456789abcdef")
	require.NoError(t, err)
	assert.ErrorContains(t, other.SignVariant(ctx, v2.String()), "SIGNING_MASTER_KEY")
}
