package unit

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/edalcin/pkd/internal/backup"
)

// memBackend is an in-memory storage.Backend implementation for restore tests.
type memBackend struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemBackend() *memBackend { return &memBackend{data: make(map[string][]byte)} }

func (b *memBackend) Put(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = buf
	_ = size
	return nil
}

func (b *memBackend) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.data[key]
	if !ok {
		return nil, 0, errNotFound{}
	}
	return io.NopCloser(bytes.NewReader(v)), int64(len(v)), nil
}

func (b *memBackend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, key)
	return nil
}

func (b *memBackend) List(_ context.Context, _ string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.data))
	for k := range b.data {
		out = append(out, k)
	}
	return out, nil
}

func (b *memBackend) Name() string { return "mem" }

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

// lookupMap is a backup.AttachmentLookup backed by an in-memory map.
type lookupMap struct {
	refs map[string][]backup.LookupRef
}

func (l *lookupMap) LookupBySHA256(_ context.Context, sha string) ([]backup.LookupRef, error) {
	return l.refs[sha], nil
}

// buildArchive composes an in-memory ZIP with the given content map and a
// matching manifest. Returns archive bytes and the SHA256s used.
func buildArchive(t *testing.T, content map[string][]byte) ([]byte, map[string][]string) {
	t.Helper()
	bySHA := make(map[string][]byte)
	storedBySHA := make(map[string][]string) // sha -> stored_filenames pointing at it
	for stored, body := range content {
		sum := sha256.Sum256(body)
		sha := hex.EncodeToString(sum[:])
		if _, exists := bySHA[sha]; !exists {
			bySHA[sha] = body
		}
		storedBySHA[sha] = append(storedBySHA[sha], stored)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for sha, body := range bySHA {
		w, _ := zw.Create(sha)
		w.Write(body)
	}
	man := &backup.Manifest{
		Version:           backup.CurrentManifestVersion,
		CreatedAt:         time.Now().UTC(),
		SourceEnvironment: backup.EnvDescriptor{BackendKind: "s3", Bucket: "test"},
	}
	for sha, body := range bySHA {
		man.Entries = append(man.Entries, backup.ManifestEntry{
			SHA256:          sha,
			SizeBytes:       int64(len(body)),
			MimeType:        "application/octet-stream",
			StoredFilenames: storedBySHA[sha],
		})
	}
	mw, _ := zw.Create(backup.ManifestFileName)
	man.Encode(mw)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes(), storedBySHA
}

func TestRestoreRoundTrip(t *testing.T) {
	contentA := []byte("alpha")
	contentB := []byte("beta beta beta")
	archive, storedBySHA := buildArchive(t, map[string][]byte{
		"keep/a/1": contentA,
		"keep/b/2": contentB,
	})

	dst := newMemBackend()
	lookup := &lookupMap{refs: make(map[string][]backup.LookupRef)}
	for sha, names := range storedBySHA {
		for _, name := range names {
			lookup.refs[sha] = append(lookup.refs[sha], backup.LookupRef{
				ID:             int64(len(lookup.refs[sha]) + 1),
				StoredFilename: name,
				MimeType:       "text/plain",
			})
		}
	}

	res, err := backup.StreamingRestore(
		context.Background(),
		bytes.NewReader(archive),
		int64(len(archive)),
		dst,
		lookup,
		backup.RestoreOptions{},
	)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Written != 2 {
		t.Errorf("written: got %d want 2", res.Written)
	}
	if res.SkippedOrphan != 0 || res.HashMismatch != 0 {
		t.Errorf("unexpected skipped: %+v", res)
	}
	// Both files present in destination.
	for _, key := range []string{"keep/a/1", "keep/b/2"} {
		rc, _, err := dst.Get(context.Background(), key)
		if err != nil {
			t.Errorf("missing %s", key)
			continue
		}
		rc.Close()
	}
}

func TestRestoreFanout(t *testing.T) {
	// Same content referenced by two attachment rows — dedup in ZIP, fan-out
	// on restore.
	body := []byte("shared")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(sha)
	w.Write(body)
	man := &backup.Manifest{
		Version: backup.CurrentManifestVersion,
		Entries: []backup.ManifestEntry{
			{SHA256: sha, SizeBytes: int64(len(body)), StoredFilenames: []string{"a/k/1", "b/k/2"}},
		},
	}
	mw, _ := zw.Create(backup.ManifestFileName)
	man.Encode(mw)
	zw.Close()

	dst := newMemBackend()
	lookup := &lookupMap{refs: map[string][]backup.LookupRef{
		sha: {
			{ID: 1, StoredFilename: "a/k/1"},
			{ID: 2, StoredFilename: "b/k/2"},
		},
	}}
	res, err := backup.StreamingRestore(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), dst, lookup, backup.RestoreOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Written != 2 {
		t.Errorf("expected 2 writes (fan-out), got %d", res.Written)
	}
	for _, key := range []string{"a/k/1", "b/k/2"} {
		if _, ok := dst.data[key]; !ok {
			t.Errorf("fan-out missed %s", key)
		}
	}
}

func TestRestoreOrphanSkipped(t *testing.T) {
	body := []byte("orphaned content")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(sha)
	w.Write(body)
	man := &backup.Manifest{
		Version: backup.CurrentManifestVersion,
		Entries: []backup.ManifestEntry{
			{SHA256: sha, SizeBytes: int64(len(body)), StoredFilenames: []string{"ghost/a/1"}},
		},
	}
	mw, _ := zw.Create(backup.ManifestFileName)
	man.Encode(mw)
	zw.Close()

	dst := newMemBackend()
	lookup := &lookupMap{refs: map[string][]backup.LookupRef{}} // empty — orphan

	res, err := backup.StreamingRestore(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), dst, lookup, backup.RestoreOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Written != 0 {
		t.Errorf("orphan should not write, got %d", res.Written)
	}
	if res.SkippedOrphan != 1 {
		t.Errorf("orphan count: got %d want 1", res.SkippedOrphan)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != "no matching attachment row" {
		t.Errorf("skipped entry mismatch: %+v", res.Skipped)
	}
}

func TestRestoreHashMismatch(t *testing.T) {
	// Manifest claims SHA256 of "good" but ZIP entry holds "bad".
	good := []byte("good content")
	goodSum := sha256.Sum256(good)
	goodSHA := hex.EncodeToString(goodSum[:])
	bad := []byte("tampered")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(goodSHA) // entry named with good SHA
	w.Write(bad)               // but content is bad
	man := &backup.Manifest{
		Version: backup.CurrentManifestVersion,
		Entries: []backup.ManifestEntry{
			{SHA256: goodSHA, SizeBytes: int64(len(good)), StoredFilenames: []string{"x/y/z"}},
		},
	}
	mw, _ := zw.Create(backup.ManifestFileName)
	man.Encode(mw)
	zw.Close()

	dst := newMemBackend()
	lookup := &lookupMap{refs: map[string][]backup.LookupRef{
		goodSHA: {{ID: 1, StoredFilename: "x/y/z"}},
	}}
	res, err := backup.StreamingRestore(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), dst, lookup, backup.RestoreOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.HashMismatch != 1 {
		t.Errorf("hash mismatch count: got %d want 1", res.HashMismatch)
	}
	if res.Written != 0 {
		t.Errorf("mismatched content must not be written")
	}
}

func TestRestoreOnConflictKeep(t *testing.T) {
	body := []byte("new content")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(sha)
	w.Write(body)
	man := &backup.Manifest{
		Version: backup.CurrentManifestVersion,
		Entries: []backup.ManifestEntry{
			{SHA256: sha, SizeBytes: int64(len(body)), StoredFilenames: []string{"k/e/y"}},
		},
	}
	mw, _ := zw.Create(backup.ManifestFileName)
	man.Encode(mw)
	zw.Close()

	dst := newMemBackend()
	dst.data["k/e/y"] = []byte("existing content")
	lookup := &lookupMap{refs: map[string][]backup.LookupRef{
		sha: {{ID: 1, StoredFilename: "k/e/y"}},
	}}

	res, err := backup.StreamingRestore(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), dst, lookup, backup.RestoreOptions{OnConflict: backup.ConflictKeep})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Kept != 1 {
		t.Errorf("kept count: got %d want 1", res.Kept)
	}
	if res.Written != 0 {
		t.Errorf("keep mode should not overwrite, written=%d", res.Written)
	}
	if string(dst.data["k/e/y"]) != "existing content" {
		t.Errorf("destination overwritten despite keep mode")
	}
}

func TestRestoreMissingManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("some-file")
	w.Write([]byte("data"))
	zw.Close()

	_, err := backup.StreamingRestore(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), newMemBackend(), &lookupMap{}, backup.RestoreOptions{})
	if err == nil || !strings.Contains(err.Error(), "manifest.json missing") {
		t.Fatalf("expected manifest missing error, got %v", err)
	}
}

func TestRestoreCorruptZip(t *testing.T) {
	_, err := backup.StreamingRestore(context.Background(), bytes.NewReader([]byte("not a zip")), 9, newMemBackend(), &lookupMap{}, backup.RestoreOptions{})
	if err == nil {
		t.Fatalf("expected error for corrupt zip")
	}
}

func TestRestoreInvalidConflictMode(t *testing.T) {
	_, err := backup.StreamingRestore(context.Background(), bytes.NewReader([]byte{}), 0, newMemBackend(), &lookupMap{}, backup.RestoreOptions{OnConflict: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "invalid on_conflict") {
		t.Fatalf("expected invalid on_conflict, got %v", err)
	}
}

// TestRestoreInPlace exercises US3: backup ZIP from a backend, simulate loss
// (delete some keys), restore the ZIP into the same backend, verify the lost
// keys come back without altering keys that were preserved.
func TestRestoreInPlace(t *testing.T) {
	contentA := []byte("alpha")
	contentB := []byte("beta")
	archive, storedBySHA := buildArchive(t, map[string][]byte{
		"keep/a/1":    contentA,
		"deleted/b/2": contentB,
	})

	// Same backend acts as source and destination (in-place).
	backend := newMemBackend()
	// Pre-populate backend as if both keys existed; then simulate loss of one.
	backend.data["keep/a/1"] = contentA
	// "deleted/b/2" intentionally missing.

	lookup := &lookupMap{refs: make(map[string][]backup.LookupRef)}
	for sha, names := range storedBySHA {
		for _, name := range names {
			lookup.refs[sha] = append(lookup.refs[sha], backup.LookupRef{
				ID:             int64(len(lookup.refs[sha]) + 1),
				StoredFilename: name,
			})
		}
	}

	res, err := backup.StreamingRestore(
		context.Background(),
		bytes.NewReader(archive),
		int64(len(archive)),
		backend,
		lookup,
		backup.RestoreOptions{OnConflict: backup.ConflictOverwrite},
	)
	if err != nil {
		t.Fatalf("restore in-place: %v", err)
	}
	if res.Written != 2 {
		t.Errorf("written: got %d want 2 (both keys re-written by overwrite)", res.Written)
	}
	if _, ok := backend.data["deleted/b/2"]; !ok {
		t.Errorf("lost key not restored in-place")
	}
	if string(backend.data["keep/a/1"]) != string(contentA) {
		t.Errorf("preserved key tampered")
	}
}

// failingBackend wraps memBackend but rejects Put for a specific key once,
// to validate per-entry failure isolation (FR-008).
type failingBackend struct {
	*memBackend
	failKey string
}

func (b *failingBackend) Put(ctx context.Context, key string, body io.Reader, size int64, mime string) error {
	if key == b.failKey {
		return errBackendDown{}
	}
	return b.memBackend.Put(ctx, key, body, size, mime)
}

type errBackendDown struct{}

func (errBackendDown) Error() string { return "backend down" }

func TestRestorePerEntryFailureIsolation(t *testing.T) {
	contentA := []byte("alpha")
	contentB := []byte("beta")
	archive, storedBySHA := buildArchive(t, map[string][]byte{
		"good/a/1": contentA,
		"bad/b/2":  contentB,
	})
	mem := newMemBackend()
	dst := &failingBackend{memBackend: mem, failKey: "bad/b/2"}

	lookup := &lookupMap{refs: make(map[string][]backup.LookupRef)}
	for sha, names := range storedBySHA {
		for _, name := range names {
			lookup.refs[sha] = append(lookup.refs[sha], backup.LookupRef{
				ID:             int64(len(lookup.refs[sha]) + 1),
				StoredFilename: name,
			})
		}
	}

	res, err := backup.StreamingRestore(context.Background(), bytes.NewReader(archive), int64(len(archive)), dst, lookup, backup.RestoreOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Written != 1 {
		t.Errorf("written: got %d want 1 (good entry only)", res.Written)
	}
	// At least one skipped record with reason mentioning the failing key.
	foundBadKey := false
	for _, s := range res.Skipped {
		if strings.Contains(s.Reason, "bad/b/2") {
			foundBadKey = true
		}
	}
	if !foundBadKey {
		t.Errorf("expected skipped entry mentioning bad/b/2, got %+v", res.Skipped)
	}
	if _, ok := mem.data["good/a/1"]; !ok {
		t.Errorf("good key should have been written despite peer failure")
	}
}
