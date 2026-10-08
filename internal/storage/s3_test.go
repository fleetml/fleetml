package storage

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Presigning for the public endpoint is local: no request is made, and both
// upload and download URLs carry the public host so clients can reach it and
// the signature (which covers the host) verifies there.
func TestPublicSigner_SignsUploadsAndDownloads(t *testing.T) {
	c, err := NewS3Client(S3Config{Endpoint: "s3:8333", AccessKey: "fleetml", SecretKey: "secret", Bucket: "models"})
	require.NoError(t, err)
	signer, err := NewPublicSigner(S3Config{AccessKey: "fleetml", SecretKey: "secret", Bucket: "models"}, "models.example.com:19000")
	require.NoError(t, err)
	c.SetPublicSigner(signer)
	for _, gen := range []func(context.Context, string, time.Duration) (string, error){c.GeneratePresignedDownload, c.GeneratePresignedUpload} {
		raw, err := gen(context.Background(), "models/p/m/1.0.0/INT8_TFLITE.bin", time.Hour)
		require.NoError(t, err)
		u, err := url.Parse(raw)
		require.NoError(t, err)
		assert.Equal(t, "models.example.com:19000", u.Host)
		assert.Equal(t, "/models/models/p/m/1.0.0/INT8_TFLITE.bin", u.Path)
		assert.Equal(t, "3600", u.Query().Get("X-Amz-Expires"))
		assert.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
	}
}
