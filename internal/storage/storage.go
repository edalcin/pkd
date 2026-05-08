package storage

import (
	"context"
	"io"
)

// Backend abstracts file storage (local filesystem or Amazon S3).
type Backend interface {
	// Put writes body to key. size must be the exact byte count of body.
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	// Get returns the file content and its size. Caller must close the reader.
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	// Delete removes key. Returns nil if the key does not exist.
	Delete(ctx context.Context, key string) error
	// List returns all logical keys with the given prefix (empty prefix = all).
	List(ctx context.Context, prefix string) ([]string, error)
	// Name returns the backend identifier: "local" or "s3".
	Name() string
}

// Seeker is an optional interface backends may implement for random-access reads.
// Backends implementing Seeker allow HTTP 206 Partial Content (range requests).
type Seeker interface {
	OpenSeek(ctx context.Context, key string) (io.ReadSeekCloser, error)
}
