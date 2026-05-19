package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
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

// UploadFromReader stores body at key using S3 multipart upload, so callers do
// not need to know the body length in advance. Used to stream a backup ZIP from
// archive/zip into S3 without buffering on the application host.
func (b *S3Backend) UploadFromReader(ctx context.Context, key string, body io.Reader, contentType string) error {
	uploader := manager.NewUploader(b.client)
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(b.bucket),
		Key:                  aws.String(b.objectKey(key)),
		Body:                 body,
		ContentType:          aws.String(contentType),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("s3 upload %q: %w", key, sanitizeAWSError(err))
	}
	return nil
}

// PresignGet returns a pre-signed GET URL that expires after ttl.
func (b *S3Backend) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	presigner := s3.NewPresignClient(b.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey(key)),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 presign %q: %w", key, sanitizeAWSError(err))
	}
	return req.URL, nil
}

// ListWithMetadata lists objects under prefix returning size and last-modified.
// The returned keys are logical (prefix-stripped) so they round-trip with
// DeleteMany.
func (b *S3Backend) ListWithMetadata(ctx context.Context, prefix string) ([]ObjectMeta, error) {
	fullPrefix := b.objectKey(prefix)
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(fullPrefix),
	})
	var out []ObjectMeta
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list metadata: %w", sanitizeAWSError(err))
		}
		for _, obj := range page.Contents {
			lk := b.logicalKey(aws.ToString(obj.Key))
			if lk == "" {
				continue
			}
			m := ObjectMeta{Key: lk}
			if obj.Size != nil {
				m.SizeBytes = *obj.Size
			}
			if obj.LastModified != nil {
				m.LastModified = *obj.LastModified
			}
			out = append(out, m)
		}
	}
	return out, nil
}

// GetRange reads length bytes from the object at key starting at offset.
// Implements storage.S3Capable for use by the restore reader, which needs
// random-access reads to locate the ZIP central directory without buffering
// the whole archive on the application host.
func (b *S3Backend) GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error) {
	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey(key)),
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get range %q: %w", key, sanitizeAWSError(err))
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read range %q: %w", key, err)
	}
	return data, nil
}

// HeadSize returns the byte size of the object at key.
func (b *S3Backend) HeadSize(ctx context.Context, key string) (int64, error) {
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey(key)),
	})
	if err != nil {
		return 0, fmt.Errorf("s3 head %q: %w", key, sanitizeAWSError(err))
	}
	if out.ContentLength == nil {
		return 0, fmt.Errorf("s3 head %q: missing content length", key)
	}
	return *out.ContentLength, nil
}

// DeleteMany removes up to len(keys) objects, batching 1000 at a time.
func (b *S3Backend) DeleteMany(ctx context.Context, keys []string) error {
	const batchSize = 1000
	for start := 0; start < len(keys); start += batchSize {
		end := start + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		ids := make([]types.ObjectIdentifier, 0, end-start)
		for _, k := range keys[start:end] {
			ids = append(ids, types.ObjectIdentifier{Key: aws.String(b.objectKey(k))})
		}
		_, err := b.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.bucket),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("s3 delete batch: %w", sanitizeAWSError(err))
		}
	}
	return nil
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
