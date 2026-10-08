package delta

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roundTrip(t *testing.T, old, next []byte) []byte {
	t.Helper()
	d, err := Diff(old, next)
	require.NoError(t, err)
	got, err := Patch(old, d)
	require.NoError(t, err)
	require.True(t, bytes.Equal(next, got), "patched output differs (old %d, next %d bytes)", len(old), len(next))
	return d
}

func TestRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	random := func(n int) []byte { b := make([]byte, n); r.Read(b); return b }

	base := random(200_000)
	cases := map[string][]byte{}
	cases["identical"] = base
	edited := append([]byte(nil), base...)
	for i := 0; i < 200; i++ { // scattered byte changes
		edited[r.Intn(len(edited))] ^= byte(r.Intn(255) + 1)
	}
	cases["scattered edits"] = edited
	cases["insertion"] = append(append(append([]byte(nil), base[:50_000]...), random(3000)...), base[50_000:]...)
	cases["deletion"] = append(append([]byte(nil), base[:70_000]...), base[90_000:]...)
	cases["last layer retrained"] = append(append([]byte(nil), base[:180_000]...), random(20_000)...)
	cases["unrelated"] = random(150_000)
	cases["empty next"] = []byte{}
	for name, next := range cases {
		t.Run(name, func(t *testing.T) { roundTrip(t, base, next) })
	}
	t.Run("empty old", func(t *testing.T) { roundTrip(t, nil, base[:1000]) })
	t.Run("tiny", func(t *testing.T) { roundTrip(t, []byte("a"), []byte("ab")) })

	d := roundTrip(t, base, cases["last layer retrained"])
	assert.Less(t, len(d), 25_000, "a delta of a mostly unchanged file is small: %d bytes", len(d))
	d = roundTrip(t, base, cases["scattered edits"])
	assert.Less(t, len(d), 10_000, "scattered edits: %d bytes", len(d))
}

func TestPatchRejectsGarbage(t *testing.T) {
	_, err := Patch(nil, []byte("not a delta"))
	assert.Error(t, err)
	old := bytes.Repeat([]byte("fleetml model weights "), 2000)
	d, err := Diff(old, append(append([]byte(nil), old...), "and a next head"...))
	require.NoError(t, err)
	_, err = Patch(old, d[:len(d)/2])
	assert.Error(t, err, "a truncated delta fails (devices also check the sha256 of the delta and of the result)")
}
