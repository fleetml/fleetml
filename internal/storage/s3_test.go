package storage

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Presigning for the public endpoint is local: no request is made, and the
// URL carries the public host so devices can reach it and the signature
// (which covers the host) verifies there.
func TestDownloadSigner_UsesPublicEndpoint(t *testing.T) {
	signer, err := NewDownloadSigner(S3Config{AccessKey: "fleetml", SecretKey: "secret", Bucket: "models"}, "10.0.2.2:19000")
	require.NoError(t, err)
	raw, err := signer.GeneratePresignedDownload(context.Background(), "models/p/m/1.0.0/INT8_TFLITE.bin", time.Hour)
	require.NoError(t, err)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "10.0.2.2:19000", u.Host)
	assert.Equal(t, "/models/models/p/m/1.0.0/INT8_TFLITE.bin", u.Path)
	assert.Equal(t, "3600", u.Query().Get("X-Amz-Expires"))
	assert.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
}
