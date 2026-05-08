package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Backend stores files in an Amazon S3 bucket.
// It does NOT implement Seeker — HTTP range requests are not supported in v1.
type S3Backend struct {
	client *s3.Client
	bucket string
	prefix string // optional key prefix (no trailing slash)
}

func NewS3(client *s3.Client, bucket, prefix string) *S3Backend {
	prefix = strings.Trim(prefix, "/")
	return &S3Backend{client: client, bucket: bucket, prefix: prefix}
}

func (b *S3Backend) Name() string { return "s3" }

func (b *S3Backend) objectKey(key string) string {
	if b.prefix == "" {
		return key
	}
	return b.prefix + "/" + key
}

func (b *S3Backend) logicalKey(objectKey string) string {
	if b.prefix == "" {
		return objectKey
	}
	return strings.TrimPrefix(objectKey, b.prefix+"/")
}

func (b *S3Backend) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(b.bucket),
		Key:                  aws.String(b.objectKey(key)),
		Body:                 body,
		ContentLength:        aws.Int64(size),
		ContentType:          aws.String(contentType),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("s3 put %q: %w", key, sanitizeAWSError(err))
	}
	return nil
}

func (b *S3Backend) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey(key)),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("s3 get %q: %w", key, sanitizeAWSError(err))
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

func (b *S3Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey(key)),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %q: %w", key, sanitizeAWSError(err))
	}
	return nil
}

func (b *S3Backend) List(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := b.objectKey(prefix)
	if b.prefix != "" && prefix == "" {
		// List everything under our bucket prefix
		fullPrefix = b.prefix + "/"
	}
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(fullPrefix),
	})
	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", sanitizeAWSError(err))
		}
		for _, obj := range page.Contents {
			lk := b.logicalKey(aws.ToString(obj.Key))
			// Skip health check objects
			if strings.Contains(lk, ".pkd-healthcheck") {
				continue
			}
			if lk != "" {
				keys = append(keys, lk)
			}
		}
	}
	return keys, nil
}

// sanitizeAWSError strips any credential or token info from AWS SDK errors.
// AWS SDK errors should not contain secrets, but we strip auth-related text as a precaution.
func sanitizeAWSError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	// Redact potential credential references
	if strings.Contains(msg, "AccessKey") || strings.Contains(msg, "SecretKey") {
		return fmt.Errorf("s3 operation failed (credential details redacted)")
	}
	return err
}
