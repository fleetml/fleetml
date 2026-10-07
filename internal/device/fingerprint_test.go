package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFingerprint(t *testing.T) {
	tests := []struct {
		name    string
		fp      string
		wantErr bool
	}{
		{"valid", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", false},
		{"too short", "abcdef123456", true},
		{"uppercase", "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2", true},
		{"non-hex", "g1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFingerprint(tt.fp)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateFingerprint(t *testing.T) {
	fp := GenerateFingerprint("ANDROID", "snapdragon-8", 12288)
	assert.Len(t, fp, 64)
	assert.NoError(t, ValidateFingerprint(fp))

	// Same inputs = same fingerprint
	fp2 := GenerateFingerprint("ANDROID", "snapdragon-8", 12288)
	assert.Equal(t, fp, fp2)

	// Different inputs = different fingerprint
	fp3 := GenerateFingerprint("IOS", "a17-pro", 8192)
	assert.NotEqual(t, fp, fp3)
}
