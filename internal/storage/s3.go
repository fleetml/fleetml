package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

type S3Client struct {
	client *minio.Client
	bucket string
	public *PublicSigner // signs client-facing URLs when the public address differs
}

func NewS3Client(cfg S3Config) (*S3Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating s3 client: %w", err)
	}

	return &S3Client{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// PublicSigner presigns URLs for the address clients (devices, the CLI,
// browsers) reach the object store at, when it differs from the one the
// server uses (e.g. a public hostname in front of a private container).
type PublicSigner struct {
	client *minio.Client
	bucket string
}

// NewPublicSigner returns a signer for publicEndpoint ("host:port").
// Presigning is computed locally (the region is fixed), so the server never
// needs to reach publicEndpoint itself.
func NewPublicSigner(cfg S3Config, publicEndpoint string) (*PublicSigner, error) {
	client, err := minio.New(publicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("creating s3 public signer: %w", err)
	}
	return &PublicSigner{client: client, bucket: cfg.Bucket}, nil
}

// SetPublicSigner makes every presigned URL point at the signer's address.
func (s *S3Client) SetPublicSigner(p *PublicSigner) { s.public = p }

// EnsureBucket creates the configured bucket if it does not exist yet, so the
// server works against a fresh S3-compatible store without an init container.
func (s *S3Client) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("checking bucket %q: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		// Another replica may have created it in the meantime.
		if code := minio.ToErrorResponse(err).Code; code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists" {
			return nil
		}
		return fmt.Errorf("creating bucket %q: %w", s.bucket, err)
	}
	slog.Info("created s3 bucket", "bucket", s.bucket)
	return nil
}

func (s *S3Client) HealthCheck(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	slog.Debug("s3 health check passed", "bucket", s.bucket)
	return nil
}

// GeneratePresignedUpload returns a pre-signed PUT URL for uploading a model blob.
func (s *S3Client) GeneratePresignedUpload(ctx context.Context, key string, expiry time.Duration) (string, error) {
	c := s.client
	if s.public != nil {
		c = s.public.client
	}
	u, err := c.PresignedPutObject(ctx, s.bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("generating presigned upload URL: %w", err)
	}
	return u.String(), nil
}

// GeneratePresignedDownload returns a pre-signed GET URL for downloading a model blob.
func (s *S3Client) GeneratePresignedDownload(ctx context.Context, key string, expiry time.Duration) (string, error) {
	c := s.client
	if s.public != nil {
		c = s.public.client
	}
	u, err := c.PresignedGetObject(ctx, s.bucket, key, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("generating presigned download URL: %w", err)
	}
	return u.String(), nil
}

// ObjectExists checks if an object exists in the bucket.
func (s *S3Client) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("checking object existence: %w", err)
	}
	return true, nil
}

// DeleteObject removes an object from the bucket.
func (s *S3Client) DeleteObject(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}
	return nil
}

// OpenObject streams an object's contents. The caller must close the reader.
func (s *S3Client) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("opening object: %w", err)
	}
	return obj, nil
}

// GetObjectInfo returns the size of an object.
func (s *S3Client) GetObjectInfo(ctx context.Context, key string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("getting object info: %w", err)
	}
	return info.Size, nil
}
