package storage

import (
	"context"
	"io"
	"time"
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

// ObjectMeta is metadata returned by S3Capable.ListWithMetadata.
type ObjectMeta struct {
	Key          string
	SizeBytes    int64
	LastModified time.Time
}

// DirPruner is an optional interface for backends that support removing
// empty subdirectories after a batch deletion. CleanupSource calls it once
// after all file deletions so that leftover empty directories are swept away.
type DirPruner interface {
	PruneEmptyDirs(ctx context.Context) error
}

// S3Capable is implemented by backends that can stream a backup ZIP entirely
// inside object storage (no temp file on the application host) and produce a
// short-lived download URL. Local backends do not implement this interface.
type S3Capable interface {
	// UploadFromReader stores body at key using multipart upload. body's length
	// does not need to be known in advance.
	UploadFromReader(ctx context.Context, key string, body io.Reader, contentType string) error

	// PresignGet returns a time-bounded URL that anyone with the URL can use to
	// download the object. ttl applies from issue time.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)

	// ListWithMetadata lists objects under prefix with their size and last
	// modification timestamp. Used by the startup sweep to find orphans.
	ListWithMetadata(ctx context.Context, prefix string) ([]ObjectMeta, error)

	// DeleteMany removes up to len(keys) objects. Implementations may batch
	// requests internally.
	DeleteMany(ctx context.Context, keys []string) error

	// GetRange reads length bytes from key starting at offset. Used to wrap
	// an S3 object as an io.ReaderAt so archive/zip can locate the central
	// directory without buffering the whole file.
	GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error)

	// HeadSize returns the total byte size of key.
	HeadSize(ctx context.Context, key string) (int64, error)
}
