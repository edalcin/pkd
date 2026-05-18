package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"sync/atomic"
	"time"

	"github.com/edalcin/pkd/internal/storage"
)

// Attachment describes one row from the attachments table for purposes of
// inclusion in a backup archive. Mirrors store.AttachmentForBackup to avoid an
// import cycle.
type Attachment struct {
	ID              int64
	StoredFilename  string
	MimeType        string
	SizeBytes       int64
	StorageLocation string
	ContentSHA256   string // may be "" when unknown — backup will compute and persist via BackfillSHA256
}

// AttachmentSource resolves an attachment's content. Implementations typically
// dispatch by Attachment.StorageLocation to either the local or S3 backend.
type AttachmentSource interface {
	Open(ctx context.Context, att Attachment) (io.ReadCloser, error)
}

// SHA256Persister is called for each attachment whose SHA256 had to be
// computed on the fly. Implementations should UPDATE the database so the hash
// is reused on subsequent backups.
type SHA256Persister interface {
	BackfillSHA256(ctx context.Context, attachmentID int64, sha256Hex string) error
}

// Result summarises a finished backup.
type Result struct {
	TotalAttachments int64
	UniqueEntries    int64
	BytesWritten     int64
}

// WriteOptions configure StreamingBackup.
type WriteOptions struct {
	Env          EnvDescriptor
	Source       AttachmentSource
	Persister    SHA256Persister
	Attachments  []Attachment
	OnProgress   func(processed int64) // called after each attachment is processed (counter increments)
}

// StreamingBackup composes a backup ZIP into sink. The ZIP body is written
// sequentially: one entry per unique content SHA256, then manifest.json. Each
// attachment is read exactly once from its source backend (or twice on the
// first run for rows whose SHA256 must be computed) and copied into the ZIP
// via TeeReader so the integrity hash matches the entry name. sink is closed
// by the caller (typically the writer half of an io.Pipe).
func StreamingBackup(ctx context.Context, sink io.Writer, opts WriteOptions) (Result, error) {
	if opts.Source == nil {
		return Result{}, fmt.Errorf("backup: Source is required")
	}
	atts := append([]Attachment(nil), opts.Attachments...)
	// Stable processing order: by ID. Keeps ZIPs reproducible for diff/audit.
	sort.Slice(atts, func(i, j int) bool { return atts[i].ID < atts[j].ID })

	// First pass: ensure every attachment has a SHA256. Rows lacking one are
	// read from source, hashed, persisted, and the hash recorded in-memory.
	// This step is independent of the second pass so the zip entry header
	// (which is immutable once written) can be created with the correct name.
	if err := ensureSHA256s(ctx, atts, opts.Source, opts.Persister); err != nil {
		return Result{}, err
	}

	// Group attachments by SHA256 (natural dedup).
	bySHA := make(map[string][]Attachment)
	order := make([]string, 0)
	for _, a := range atts {
		if _, exists := bySHA[a.SHA256OrZero()]; !exists {
			order = append(order, a.SHA256OrZero())
		}
		bySHA[a.SHA256OrZero()] = append(bySHA[a.SHA256OrZero()], a)
	}

	zw := zip.NewWriter(sink)
	var bytesWritten int64
	var processed int64

	for _, sha := range order {
		bucket := bySHA[sha]
		// One ZIP entry per unique content. Use the first attachment in the
		// group to read content from (any would do — they share SHA256).
		src := bucket[0]

		hdr := &zip.FileHeader{
			Name:     sha,
			Method:   zip.Store, // attachments are mostly already-compressed binaries (pdf/jpg/zip)
			Modified: time.Now().UTC(),
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return Result{}, fmt.Errorf("backup: zip header %s: %w", sha, err)
		}

		rc, err := opts.Source.Open(ctx, src)
		if err != nil {
			return Result{}, fmt.Errorf("backup: open %s: %w", src.StoredFilename, err)
		}

		h := sha256.New()
		tee := io.TeeReader(rc, h)
		n, err := io.Copy(w, tee)
		rc.Close()
		if err != nil {
			return Result{}, fmt.Errorf("backup: copy %s: %w", src.StoredFilename, err)
		}
		got := hex.EncodeToString(h.Sum(nil))
		if got != sha {
			return Result{}, fmt.Errorf("backup: integrity check failed for %s (expected %s, got %s)", src.StoredFilename, sha, got)
		}
		bytesWritten += n

		// Progress increments by the number of attachment rows that share this
		// content — caller sees one "tick" per attachment, not per dedup group.
		for range bucket {
			atomic.AddInt64(&processed, 1)
			if opts.OnProgress != nil {
				opts.OnProgress(atomic.LoadInt64(&processed))
			}
		}
	}

	// Manifest as last entry.
	man := &Manifest{
		Version:           CurrentManifestVersion,
		CreatedAt:         time.Now().UTC(),
		SourceEnvironment: opts.Env,
	}
	for _, sha := range order {
		bucket := bySHA[sha]
		entry := ManifestEntry{
			SHA256:    sha,
			SizeBytes: bucket[0].SizeBytes,
			MimeType:  bucket[0].MimeType,
		}
		for _, a := range bucket {
			entry.StoredFilenames = append(entry.StoredFilenames, a.StoredFilename)
		}
		man.Entries = append(man.Entries, entry)
	}
	mw, err := zw.Create(ManifestFileName)
	if err != nil {
		return Result{}, fmt.Errorf("backup: manifest header: %w", err)
	}
	if err := man.Encode(mw); err != nil {
		return Result{}, fmt.Errorf("backup: manifest encode: %w", err)
	}
	if err := zw.Close(); err != nil {
		return Result{}, fmt.Errorf("backup: zip close: %w", err)
	}

	return Result{
		TotalAttachments: int64(len(atts)),
		UniqueEntries:    int64(len(order)),
		BytesWritten:     bytesWritten,
	}, nil
}

// SHA256OrZero returns ContentSHA256 or a sentinel that ensures attachments
// missing a hash group together (they should have been filled by
// ensureSHA256s before this is called).
func (a Attachment) SHA256OrZero() string {
	if a.ContentSHA256 == "" {
		return "00000000000000000000000000000000000000000000000000000000pending"
	}
	return a.ContentSHA256
}

// ensureSHA256s walks atts, computes hashes for entries lacking one, and
// persists them. Mutates the slice in place.
func ensureSHA256s(ctx context.Context, atts []Attachment, src AttachmentSource, persister SHA256Persister) error {
	for i := range atts {
		if atts[i].ContentSHA256 != "" {
			continue
		}
		rc, err := src.Open(ctx, atts[i])
		if err != nil {
			return fmt.Errorf("backup: open for sha %s: %w", atts[i].StoredFilename, err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, rc); err != nil {
			rc.Close()
			return fmt.Errorf("backup: hash %s: %w", atts[i].StoredFilename, err)
		}
		rc.Close()
		sum := hex.EncodeToString(h.Sum(nil))
		atts[i].ContentSHA256 = sum
		if persister != nil {
			if err := persister.BackfillSHA256(ctx, atts[i].ID, sum); err != nil {
				return fmt.Errorf("backup: persist sha %d: %w", atts[i].ID, err)
			}
		}
	}
	return nil
}

// Ensure storage interface is exercised when this package is imported.
// (no-op reference to silence unused-import warnings during incremental edits)
var _ = storage.ObjectMeta{}
