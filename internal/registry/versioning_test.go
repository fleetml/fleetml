package registry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Semver
		wantErr bool
	}{
		{name: "valid", input: "1.2.3", want: Semver{1, 2, 3}},
		{name: "with v prefix", input: "v1.0.0", want: Semver{1, 0, 0}},
		{name: "zero version", input: "0.0.0", want: Semver{0, 0, 0}},
		{name: "large numbers", input: "100.200.300", want: Semver{100, 200, 300}},
		{name: "too few parts", input: "1.2", wantErr: true},
		{name: "too many parts", input: "1.2.3.4", wantErr: true},
		{name: "non-numeric", input: "a.b.c", wantErr: true},
		{name: "negative", input: "-1.0.0", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "prerelease", input: "1.0.0-rc.1", want: Semver{1, 0, 0}},
		{name: "build metadata", input: "1.0.0+build", want: Semver{1, 0, 0}},
		{name: "prerelease and build", input: "v2.1.0-alpha-2.0+exp.sha.5114f85", want: Semver{2, 1, 0}},
		{name: "leading zero major", input: "01.0.0", wantErr: true},
		{name: "leading zero minor", input: "1.00.0", wantErr: true},
		{name: "empty prerelease", input: "1.0.0-", wantErr: true},
		{name: "prerelease leading zero", input: "1.0.0-01", wantErr: true},
		{name: "empty prerelease identifier", input: "1.0.0-rc..1", wantErr: true},
		{name: "empty build", input: "1.0.0+", wantErr: true},
		{name: "bad prerelease character", input: "1.0.0-rc_1", wantErr: true},
		{name: "plus sign number", input: "+1.0.0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSemver(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSemver_String(t *testing.T) {
	assert.Equal(t, "1.2.3", Semver{1, 2, 3}.String())
	assert.Equal(t, "0.0.0", Semver{0, 0, 0}.String())
}

func TestSemver_LessThan(t *testing.T) {
	tests := []struct {
		name string
		a, b Semver
		want bool
	}{
		{name: "major less", a: Semver{1, 0, 0}, b: Semver{2, 0, 0}, want: true},
		{name: "major greater", a: Semver{2, 0, 0}, b: Semver{1, 0, 0}, want: false},
		{name: "minor less", a: Semver{1, 1, 0}, b: Semver{1, 2, 0}, want: true},
		{name: "patch less", a: Semver{1, 1, 1}, b: Semver{1, 1, 2}, want: true},
		{name: "equal", a: Semver{1, 1, 1}, b: Semver{1, 1, 1}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.LessThan(tt.b))
		})
	}
}

func TestNextVersions(t *testing.T) {
	patch, err := NextPatch("1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "1.2.4", patch)

	minor, err := NextMinor("1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "1.3.0", minor)

	major, err := NextMajor("1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", major)
}

// The message names the version as it was given, "v" included.
func TestParseSemver_MessageNamesTheInput(t *testing.T) {
	for _, in := range []string{"v1", "01.0.0", "1.0.0-01", "1.0.0+"} {
		_, err := ParseSemver(in)
		if err == nil || !strings.Contains(err.Error(), `invalid semver "`+in+`"`) {
			t.Fatalf("%s: got %v", in, err)
		}
	}
}
