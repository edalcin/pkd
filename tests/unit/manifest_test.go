package unit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/edalcin/pkd/internal/backup"
)

func TestManifestRoundTrip(t *testing.T) {
	orig := &backup.Manifest{
		Version: backup.CurrentManifestVersion,
		SourceEnvironment: backup.EnvDescriptor{
			BackendKind: "s3",
			Bucket:      "pkd-prod",
			Prefix:      "attachments/",
		},
		Entries: []backup.ManifestEntry{
			{
				SHA256:          strings.Repeat("a", 64),
				SizeBytes:       1024,
				MimeType:        "application/pdf",
				StoredFilenames: []string{"ab/cd/abc123", "ef/01/def456"},
			},
		},
	}
	var buf bytes.Buffer
	if err := orig.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := backup.DecodeManifest(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != orig.Version {
		t.Errorf("version: got %d want %d", got.Version, orig.Version)
	}
	if len(got.Entries) != 1 || got.Entries[0].SHA256 != orig.Entries[0].SHA256 {
		t.Errorf("entries mismatch: got %+v", got.Entries)
	}
	if len(got.Entries[0].StoredFilenames) != 2 {
		t.Errorf("expected 2 stored_filenames, got %d", len(got.Entries[0].StoredFilenames))
	}
}

func TestManifestRejectsWrongVersion(t *testing.T) {
	bad := strings.NewReader(`{"version":2,"created_at":"2026-05-18T00:00:00Z","source_environment":{"backend_kind":"s3"},"entries":[]}`)
	_, err := backup.DecodeManifest(bad)
	if err == nil || !strings.Contains(err.Error(), "version 2 not supported") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestManifestRejectsBadSHA(t *testing.T) {
	bad := strings.NewReader(`{"version":1,"created_at":"2026-05-18T00:00:00Z","source_environment":{"backend_kind":"s3"},"entries":[{"sha256":"not-a-hash","size_bytes":1,"stored_filenames":["x"]}]}`)
	_, err := backup.DecodeManifest(bad)
	if err == nil || !strings.Contains(err.Error(), "invalid sha256") {
		t.Fatalf("expected sha error, got %v", err)
	}
}

func TestManifestRejectsEmptyStoredFilenames(t *testing.T) {
	bad := strings.NewReader(`{"version":1,"created_at":"2026-05-18T00:00:00Z","source_environment":{"backend_kind":"s3"},"entries":[{"sha256":"` + strings.Repeat("a", 64) + `","size_bytes":1,"stored_filenames":[]}]}`)
	_, err := backup.DecodeManifest(bad)
	if err == nil || !strings.Contains(err.Error(), "empty stored_filenames") {
		t.Fatalf("expected empty stored_filenames error, got %v", err)
	}
}
