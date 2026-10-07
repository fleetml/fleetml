package storage

import (
	"context"
	"fmt"
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
	u, err := s.client.PresignedPutObject(ctx, s.bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("generating presigned upload URL: %w", err)
	}
	return u.String(), nil
}

// GeneratePresignedDownload returns a pre-signed GET URL for downloading a model blob.
func (s *S3Client) GeneratePresignedDownload(ctx context.Context, key string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, reqParams)
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

// GetObjectInfo returns the size of an object.
func (s *S3Client) GetObjectInfo(ctx context.Context, key string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("getting object info: %w", err)
	}
	return info.Size, nil
}
