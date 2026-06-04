package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/edalcin/pkd/internal/security"
)

// LocalBackend stores files on the local filesystem under basePath.
// It implements both Backend and Seeker (HTTP range requests supported).
type LocalBackend struct {
	basePath string
}

func NewLocal(basePath string) *LocalBackend {
	return &LocalBackend{basePath: basePath}
}

func (b *LocalBackend) Name() string { return "local" }

func (b *LocalBackend) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	path, err := security.SafeAttachmentPath(b.basePath, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("storage/local mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("storage/local create: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		os.Remove(path)
		return fmt.Errorf("storage/local write: %w", err)
	}
	return nil
}

func (b *LocalBackend) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := security.SafeAttachmentPath(b.basePath, key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("storage/local open: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("storage/local stat: %w", err)
	}
	return f, fi.Size(), nil
}

// OpenSeek implements Seeker for HTTP range request support.
func (b *LocalBackend) OpenSeek(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	path, err := security.SafeAttachmentPath(b.basePath, key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("storage/local open: %w", err)
	}
	return f, nil
}

func (b *LocalBackend) Delete(ctx context.Context, key string) error {
	path, err := security.SafeAttachmentPath(b.basePath, key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// Remove empty parent directories up to (not including) basePath.
	dir := filepath.Dir(path)
	for dir != b.basePath && len(dir) > len(b.basePath) {
		if rmErr := os.Remove(dir); rmErr != nil {
			break // not empty or other error — stop
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// PruneEmptyDirs removes empty subdirectories under basePath, deepest first.
// Called after a batch deletion to sweep up any leftover empty directories that
// per-file cleanup missed (e.g. when sibling files were skipped, not deleted).
func (b *LocalBackend) PruneEmptyDirs(_ context.Context) error {
	var dirs []string
	filepath.Walk(b.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || path == b.basePath {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	// Sort deepest-first by path length so children are attempted before parents.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		os.Remove(dir) // best-effort: silently fails if not empty
	}
	return nil
}

func (b *LocalBackend) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.Walk(b.basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.basePath, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}
