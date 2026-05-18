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

	"github.com/edalcin/pkd/internal/backup"
)

// fakeSource serves attachment content from an in-memory map keyed by
// stored filename.
type fakeSource struct {
	data map[string][]byte
}

func (f *fakeSource) Open(_ context.Context, att backup.Attachment) (io.ReadCloser, error) {
	b, ok := f.data[att.StoredFilename]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

type recordingPersister struct {
	mu       sync.Mutex
	backfill map[int64]string
}

func (r *recordingPersister) BackfillSHA256(_ context.Context, id int64, sha string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.backfill == nil {
		r.backfill = make(map[int64]string)
	}
	r.backfill[id] = sha
	return nil
}

func TestStreamingBackupRoundTrip(t *testing.T) {
	contentA := []byte("file A content")
	contentB := []byte("file B content — distinct")
	shaA := sumHex(contentA)
	shaB := sumHex(contentB)

	src := &fakeSource{data: map[string][]byte{
		"a/keep/one": contentA,
		"b/keep/two": contentB,
		"a/keep/dup": contentA, // dedup target
	}}
	persister := &recordingPersister{}
	atts := []backup.Attachment{
		{ID: 1, StoredFilename: "a/keep/one", MimeType: "text/plain", SizeBytes: int64(len(contentA)), StorageLocation: "s3", ContentSHA256: shaA},
		{ID: 2, StoredFilename: "b/keep/two", MimeType: "text/plain", SizeBytes: int64(len(contentB)), StorageLocation: "s3", ContentSHA256: ""}, // forces backfill
		{ID: 3, StoredFilename: "a/keep/dup", MimeType: "text/plain", SizeBytes: int64(len(contentA)), StorageLocation: "s3", ContentSHA256: shaA},
	}

	var buf bytes.Buffer
	res, err := backup.StreamingBackup(context.Background(), &buf, backup.WriteOptions{
		Env:         backup.EnvDescriptor{BackendKind: "s3", Bucket: "test"},
		Source:      src,
		Persister:   persister,
		Attachments: atts,
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if res.TotalAttachments != 3 {
		t.Errorf("total: got %d want 3", res.TotalAttachments)
	}
	if res.UniqueEntries != 2 {
		t.Errorf("unique: got %d want 2 (dedup expected)", res.UniqueEntries)
	}

	// Backfill should have persisted SHA for row 2 only.
	if got, ok := persister.backfill[2]; !ok || got != shaB {
		t.Errorf("backfill[2] missing or wrong: %v", persister.backfill)
	}
	if _, ok := persister.backfill[1]; ok {
		t.Errorf("backfill[1] should not happen (already had SHA)")
	}

	// Open the produced ZIP and verify structure.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip read: %v", err)
	}
	entryNames := make(map[string]bool)
	var manifestBytes []byte
	for _, f := range zr.File {
		entryNames[f.Name] = true
		if f.Name == backup.ManifestFileName {
			rc, _ := f.Open()
			manifestBytes, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if !entryNames[shaA] || !entryNames[shaB] {
		t.Errorf("missing content entry; have %v", entryNames)
	}
	if !entryNames[backup.ManifestFileName] {
		t.Errorf("manifest missing from zip")
	}
	man, err := backup.DecodeManifest(bytes.NewReader(manifestBytes))
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(man.Entries) != 2 {
		t.Fatalf("manifest entries: got %d want 2", len(man.Entries))
	}
	// Find entry for shaA — must list both stored_filenames (dedup).
	for _, e := range man.Entries {
		if e.SHA256 == shaA {
			if len(e.StoredFilenames) != 2 {
				t.Errorf("dedup entry should list 2 filenames, got %v", e.StoredFilenames)
			}
		}
	}
}

func TestStreamingBackupNoSource(t *testing.T) {
	_, err := backup.StreamingBackup(context.Background(), io.Discard, backup.WriteOptions{
		Attachments: []backup.Attachment{},
	})
	if err == nil || !strings.Contains(err.Error(), "Source is required") {
		t.Fatalf("expected Source error, got %v", err)
	}
}

func sumHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
