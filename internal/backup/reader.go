package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/edalcin/pkd/internal/storage"
)

// Conflict resolution modes for restore.
const (
	ConflictOverwrite = "overwrite" // default — restore replaces existing object
	ConflictKeep      = "keep"      // skip if destination already has the key
	ConflictAbort     = "abort"     // first conflict aborts entire restore
)

// RestoreOptions tunes StreamingRestore behaviour.
type RestoreOptions struct {
	// OnConflict determines what happens when an attachment's StoredFilename
	// already exists in the destination backend. Defaults to "overwrite".
	OnConflict string
	// OnProgress is called after each attachment row is written. counter is
	// monotonically increasing, including skipped/kept entries.
	OnProgress func(processed int64)
}

// AttachmentLookup resolves manifest entries to live attachment rows by
// SHA256. Returned slice is empty when no row references that content
// (orphan entry — restore skips per FR-017).
type AttachmentLookup interface {
	LookupBySHA256(ctx context.Context, sha256Hex string) ([]LookupRef, error)
}

// LookupRef mirrors store.AttachmentRef to avoid import cycle.
type LookupRef struct {
	ID              int64
	StoredFilename  string
	StorageLocation string
	MimeType        string
}

// SkippedEntry records a manifest entry that restore did not write to the
// destination backend. Surfaced in the job result so the operator can audit.
type SkippedEntry struct {
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	Reason    string `json:"reason"`
}

// RestoreResult summarises what restore did.
type RestoreResult struct {
	ManifestEntries int64
	Written         int64 // backend Put calls that succeeded
	Kept            int64 // destination already had the key and OnConflict=keep
	SkippedOrphan   int64 // no attachment row matched the SHA256
	HashMismatch    int64 // content hash did not match manifest entry name
	BytesWritten    int64
	Skipped         []SkippedEntry
}

// StreamingRestore reads a backup ZIP from zipSrc, decodes its manifest,
// looks up destination rows by SHA256, and writes each entry into dst.
// Returns a RestoreResult and a non-nil error only for fatal conditions
// (manifest missing, corrupt ZIP, conflict abort). Per-entry failures
// (hash mismatch, orphan, individual Put error) are recorded in the result
// without aborting the whole operation, per FR-008.
func StreamingRestore(
	ctx context.Context,
	zipSrc io.ReaderAt,
	size int64,
	dst storage.Backend,
	lookup AttachmentLookup,
	opts RestoreOptions,
) (RestoreResult, error) {
	if opts.OnConflict == "" {
		opts.OnConflict = ConflictOverwrite
	}
	switch opts.OnConflict {
	case ConflictOverwrite, ConflictKeep, ConflictAbort:
	default:
		return RestoreResult{}, fmt.Errorf("invalid on_conflict %q", opts.OnConflict)
	}

	zr, err := zip.NewReader(zipSrc, size)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("restore: open zip: %w", err)
	}

	// Locate manifest. Indexed by name for O(1) lookup of content entries
	// below.
	var manifestFile *zip.File
	entriesByName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if f.Name == ManifestFileName {
			manifestFile = f
			continue
		}
		entriesByName[f.Name] = f
	}
	if manifestFile == nil {
		return RestoreResult{}, fmt.Errorf("restore: %s missing from archive", ManifestFileName)
	}

	mrc, err := manifestFile.Open()
	if err != nil {
		return RestoreResult{}, fmt.Errorf("restore: open manifest: %w", err)
	}
	manifest, err := DecodeManifest(mrc)
	mrc.Close()
	if err != nil {
		return RestoreResult{}, fmt.Errorf("restore: decode manifest: %w", err)
	}

	res := RestoreResult{ManifestEntries: int64(len(manifest.Entries))}
	var processed int64

	for _, entry := range manifest.Entries {
		refs, err := lookup.LookupBySHA256(ctx, entry.SHA256)
		if err != nil {
			return res, fmt.Errorf("restore: lookup sha %s: %w", entry.SHA256, err)
		}
		if len(refs) == 0 {
			res.SkippedOrphan++
			res.Skipped = append(res.Skipped, SkippedEntry{
				SHA256:    entry.SHA256,
				SizeBytes: entry.SizeBytes,
				Reason:    "no matching attachment row",
			})
			continue
		}

		zf, ok := entriesByName[entry.SHA256]
		if !ok {
			res.Skipped = append(res.Skipped, SkippedEntry{
				SHA256:    entry.SHA256,
				SizeBytes: entry.SizeBytes,
				Reason:    "manifest references entry missing from archive",
			})
			continue
		}

		// Buffer the entry. ZIP entries are decompressed lazily; we need the
		// bytes both to verify SHA256 and (potentially) to fan out to multiple
		// destination keys.
		rc, err := zf.Open()
		if err != nil {
			res.Skipped = append(res.Skipped, SkippedEntry{
				SHA256:    entry.SHA256,
				SizeBytes: entry.SizeBytes,
				Reason:    fmt.Sprintf("zip open: %v", err),
			})
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			res.Skipped = append(res.Skipped, SkippedEntry{
				SHA256:    entry.SHA256,
				SizeBytes: entry.SizeBytes,
				Reason:    fmt.Sprintf("zip read: %v", err),
			})
			continue
		}
		sum := sha256.Sum256(content)
		got := hex.EncodeToString(sum[:])
		if got != entry.SHA256 {
			res.HashMismatch++
			res.Skipped = append(res.Skipped, SkippedEntry{
				SHA256:    entry.SHA256,
				SizeBytes: entry.SizeBytes,
				Reason:    fmt.Sprintf("integrity: content hash %s does not match", got),
			})
			continue
		}

		// Fan out: write content to every stored_filename in the refs list.
		for _, ref := range refs {
			if opts.OnConflict != ConflictOverwrite {
				_, _, getErr := dst.Get(ctx, ref.StoredFilename)
				exists := getErr == nil
				if exists {
					switch opts.OnConflict {
					case ConflictKeep:
						res.Kept++
						processed++
						if opts.OnProgress != nil {
							opts.OnProgress(processed)
						}
						continue
					case ConflictAbort:
						return res, fmt.Errorf("restore: conflict on %q (on_conflict=abort)", ref.StoredFilename)
					}
				}
			}

			mime := ref.MimeType
			if mime == "" {
				mime = entry.MimeType
			}
			if mime == "" {
				mime = "application/octet-stream"
			}
			if err := dst.Put(ctx, ref.StoredFilename, bytes.NewReader(content), int64(len(content)), mime); err != nil {
				res.Skipped = append(res.Skipped, SkippedEntry{
					SHA256:    entry.SHA256,
					SizeBytes: entry.SizeBytes,
					Reason:    fmt.Sprintf("put %s: %v", ref.StoredFilename, err),
				})
				continue
			}
			res.Written++
			res.BytesWritten += int64(len(content))
			processed++
			if opts.OnProgress != nil {
				opts.OnProgress(processed)
			}
		}
	}

	return res, nil
}

// S3RangeReaderAt adapts an S3-resident object to io.ReaderAt by issuing
// Range GET requests on every Read call. Used by restore on the S3 backend
// to feed zip.NewReader without buffering the entire archive on the
// application host.
type S3RangeReaderAt struct {
	ctx context.Context
	cap storage.S3Capable
	key string
}

// NewS3RangeReaderAt builds a reader for key. ctx applies to every Range GET.
func NewS3RangeReaderAt(ctx context.Context, cap storage.S3Capable, key string) *S3RangeReaderAt {
	return &S3RangeReaderAt{ctx: ctx, cap: cap, key: key}
}

// ReadAt satisfies io.ReaderAt. Each call costs one S3 GetObject request
// with a Range header. archive/zip issues a small number of these to locate
// the central directory plus one per file body — fine for restore which is
// not latency-sensitive.
func (r *S3RangeReaderAt) ReadAt(p []byte, off int64) (int, error) {
	data, err := r.cap.GetRange(r.ctx, r.key, off, int64(len(p)))
	if err != nil {
		return 0, err
	}
	n := copy(p, data)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
