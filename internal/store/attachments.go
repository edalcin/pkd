package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/storage"
)

// AttachmentStore manages attachment metadata in SQLite and delegates file I/O
// to storage.Backend implementations.
type AttachmentStore struct {
	db    *sql.DB
	local storage.Backend
	s3    storage.Backend // nil when S3 is not configured
}

// NewAttachmentStore returns an AttachmentStore. s3 may be nil.
func NewAttachmentStore(db *sql.DB, local storage.Backend, s3 storage.Backend) *AttachmentStore {
	return &AttachmentStore{db: db, local: local, s3: s3}
}

// BackendForLocation returns the correct backend for the given storage_location value.
// Falls back to local if the named backend is unavailable.
func (s *AttachmentStore) BackendForLocation(location string) storage.Backend {
	if location == "s3" && s.s3 != nil {
		return s.s3
	}
	return s.local
}

// ReservedBackupPrefix is the storage key prefix reserved for transient backup
// artifacts. Real attachment keys must never start with this prefix.
const ReservedBackupPrefix = "_backup-tmp/"

// CreateFile buffers body (enforcing maxBytes), writes to backend, and inserts
// the metadata row in SQLite. Returns the created Attachment with URL populated.
// subdir adds an extra path level (e.g. "inline" for editor-embedded images).
func (s *AttachmentStore) CreateFile(ctx context.Context, backend storage.Backend, docID int64, origName, mimeType, subdir string, body io.Reader, maxBytes int64) (*model.Attachment, error) {
	// Defense in depth: reject subdir that would collide with the reserved
	// backup prefix. Token-derived keys cannot collide, but a future caller
	// passing user-controlled subdir must not be able to write into the
	// transient backup namespace.
	if subdir != "" && (subdir == "_backup-tmp" || strings.HasPrefix(subdir, "_backup-tmp/")) {
		return nil, fmt.Errorf("subdir %q is reserved", subdir)
	}

	// Buffer the body to enforce size limit and get the exact byte count for S3.
	buf, n, err := readWithLimit(body, maxBytes)
	if err != nil {
		return nil, err
	}

	// Generate a random logical key: [subdir/]ab/cd/<token>
	token := security.NewToken(16)
	var key string
	if subdir != "" {
		key = fmt.Sprintf("%s/%s/%s/%s", subdir, token[:2], token[2:4], token)
	} else {
		key = fmt.Sprintf("%s/%s/%s", token[:2], token[2:4], token)
	}

	// Compute SHA256 for migration verification (stored on S3 uploads; optional on local).
	sum := sha256.Sum256(buf)
	checksum := hex.EncodeToString(sum[:])

	if err := backend.Put(ctx, key, bytes.NewReader(buf), n, mimeType); err != nil {
		return nil, fmt.Errorf("storage put: %w", err)
	}

	att := &model.Attachment{
		DocumentID:      docID,
		OriginalName:    origName,
		StoredFilename:  key,
		MimeType:        mimeType,
		SizeBytes:       n,
		StorageLocation: backend.Name(),
		ContentSHA256:   checksum,
	}

	err = WithTx(s.db, func(tx *sql.Tx) error {
		res, err := tx.Exec(`
			INSERT INTO attachments (document_id, original_name, stored_filename, mime_type, size_bytes, storage_location, content_sha256, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			att.DocumentID, att.OriginalName, att.StoredFilename, att.MimeType, att.SizeBytes,
			att.StorageLocation, att.ContentSHA256,
			time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		att.ID = id
		att.URL = fmt.Sprintf("/api/attachments/%d", id)
		return nil
	})
	if err != nil {
		backend.Delete(ctx, key) // best-effort rollback
		return nil, err
	}

	return att, nil
}

// GetByID returns the attachment metadata. Returns ErrNotFound if missing.
func (s *AttachmentStore) GetByID(id int64) (*model.Attachment, error) {
	var att model.Attachment
	var createdStr string
	var sha sql.NullString
	err := s.db.QueryRow(`
		SELECT id, document_id, original_name, stored_filename, mime_type, size_bytes, storage_location, content_sha256, created_at
		FROM attachments WHERE id = ?`, id).Scan(
		&att.ID, &att.DocumentID, &att.OriginalName, &att.StoredFilename,
		&att.MimeType, &att.SizeBytes, &att.StorageLocation, &sha, &createdStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	att.ContentSHA256 = sha.String
	att.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	att.URL = fmt.Sprintf("/api/attachments/%d", att.ID)
	return &att, nil
}

// Delete removes the file from its storage backend and the metadata row.
func (s *AttachmentStore) Delete(id int64) error {
	att, err := s.GetByID(id)
	if err != nil {
		return err
	}
	backend := s.BackendForLocation(att.StorageLocation)
	backend.Delete(context.Background(), att.StoredFilename) // best-effort
	_, err = s.db.Exec(`DELETE FROM attachments WHERE id = ?`, id)
	return err
}

// ListByDocument returns all attachments for a document.
func (s *AttachmentStore) ListByDocument(docID int64) ([]*model.Attachment, error) {
	rows, err := s.db.Query(`
		SELECT id, document_id, original_name, stored_filename, mime_type, size_bytes, storage_location, created_at
		FROM attachments WHERE document_id = ? ORDER BY created_at`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var atts []*model.Attachment
	for rows.Next() {
		var att model.Attachment
		var createdStr string
		if err := rows.Scan(&att.ID, &att.DocumentID, &att.OriginalName, &att.StoredFilename,
			&att.MimeType, &att.SizeBytes, &att.StorageLocation, &createdStr); err != nil {
			return nil, err
		}
		att.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		att.URL = fmt.Sprintf("/api/attachments/%d", att.ID)
		atts = append(atts, &att)
	}
	return atts, rows.Err()
}

// ListAllWithDocument returns all attachments joined with their document title,
// ordered newest first. Used by the admin attachments panel.
func (s *AttachmentStore) ListAllWithDocument() ([]*model.AttachmentWithDoc, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.document_id, a.original_name, a.mime_type, a.size_bytes, a.storage_location, a.created_at,
		       COALESCE(d.title, '(sem documento)') as doc_title
		FROM attachments a
		LEFT JOIN documents d ON d.id = a.document_id
		ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var atts []*model.AttachmentWithDoc
	for rows.Next() {
		var a model.AttachmentWithDoc
		var createdStr string
		if err := rows.Scan(&a.ID, &a.DocumentID, &a.OriginalName, &a.MimeType, &a.SizeBytes,
			&a.StorageLocation, &createdStr, &a.DocumentTitle); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		a.URL = fmt.Sprintf("/api/attachments/%d", a.ID)
		atts = append(atts, &a)
	}
	return atts, rows.Err()
}

// DeleteByDocument removes all attachment files and rows for a given document.
// Used before permanently deleting a document.
func (s *AttachmentStore) DeleteByDocument(docID int64) error {
	atts, err := s.ListByDocument(docID)
	if err != nil {
		return err
	}
	for _, att := range atts {
		backend := s.BackendForLocation(att.StorageLocation)
		backend.Delete(context.Background(), att.StoredFilename) // best-effort
	}
	_, err = s.db.Exec(`DELETE FROM attachments WHERE document_id = ?`, docID)
	return err
}

// ListOrphanedStoredFiles returns logical keys that exist in the active backend
// storage but have no matching row in the database.
// Only checks the local backend (orphan detection for S3 runs via admin migrate endpoint).
func (s *AttachmentStore) ListOrphanedStoredFiles() ([]string, error) {
	rows, err := s.db.Query(`SELECT stored_filename FROM attachments WHERE storage_location = 'local'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := make(map[string]struct{})
	for rows.Next() {
		var sf string
		if err := rows.Scan(&sf); err != nil {
			return nil, err
		}
		known[sf] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	diskKeys, err := s.local.List(context.Background(), "")
	if err != nil {
		return nil, err
	}

	var orphans []string
	for _, key := range diskKeys {
		if _, ok := known[key]; !ok {
			orphans = append(orphans, key)
		}
	}
	return orphans, nil
}

// DanglingAttachment is an attachment whose document is in the trash or missing.
type DanglingAttachment struct {
	ID              int64
	StoredFilename  string
	OriginalName    string
	SizeBytes       int64
	StorageLocation string
	DocTrashed      bool   // true when document exists but is in trash
	DocID           int64  // 0 when document doesn't exist
	DocTitle        string // empty when document doesn't exist
}

// ListDanglingAttachments returns attachments whose document either does not
// exist or is in the trash. These files are inaccessible through normal UI.
func (s *AttachmentStore) ListDanglingAttachments() ([]*DanglingAttachment, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.stored_filename, a.original_name, a.size_bytes, a.storage_location,
		       d.trashed_at IS NOT NULL AS doc_trashed,
		       COALESCE(d.id, 0) AS doc_id,
		       COALESCE(d.title, '') AS doc_title
		FROM attachments a
		LEFT JOIN documents d ON d.id = a.document_id
		WHERE d.id IS NULL OR d.trashed_at IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*DanglingAttachment
	for rows.Next() {
		da := &DanglingAttachment{}
		var docTrashed int
		if err := rows.Scan(&da.ID, &da.StoredFilename, &da.OriginalName, &da.SizeBytes, &da.StorageLocation, &docTrashed, &da.DocID, &da.DocTitle); err != nil {
			return nil, err
		}
		da.DocTrashed = docTrashed == 1
		result = append(result, da)
	}
	return result, rows.Err()
}

// ReconcileStorageLocations scans src bucket and, for each key that matches a
// stored_filename in the DB but has a different storage_location, updates the DB
// to reflect where the file actually lives. Returns (fixed, errors).
// Use this when the DB storage_location is stale (e.g. after a DB restore).
func (s *AttachmentStore) ReconcileStorageLocations(ctx context.Context, src storage.Backend) (int, []string) {
	if src == nil {
		return 0, []string{"source backend not configured"}
	}
	keys, err := src.List(ctx, "")
	if err != nil {
		return 0, []string{fmt.Sprintf("list %s: %v", src.Name(), err)}
	}

	fixed := 0
	var errs []string
	for _, key := range keys {
		var id int64
		var curLoc string
		err := s.db.QueryRowContext(ctx,
			`SELECT id, storage_location FROM attachments WHERE stored_filename = ?`, key).
			Scan(&id, &curLoc)
		if errors.Is(err, sql.ErrNoRows) {
			continue // orphan in bucket — not our concern here
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: db query: %v", key, err))
			continue
		}
		if curLoc == src.Name() {
			continue // already correct
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE attachments SET storage_location = ? WHERE id = ?`, src.Name(), id); err != nil {
			errs = append(errs, fmt.Sprintf("%s: db update: %v", key, err))
			continue
		}
		fixed++
	}
	return fixed, errs
}

// MigrateToBackend copies all attachments with storage_location != target to the target backend,
// verifying SHA256 integrity. Non-destructive: source files are not deleted.
// Returns (total_found, copied, errors).
func (s *AttachmentStore) MigrateToBackend(ctx context.Context, target storage.Backend) (int, int, []string) {
	rows, err := s.db.Query(`
		SELECT id, stored_filename, mime_type, storage_location, content_sha256
		FROM attachments WHERE storage_location != ?`, target.Name())
	if err != nil {
		return 0, 0, []string{fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	type row struct {
		id       int64
		key      string
		mime     string
		location string
		sha256   string
	}
	var todo []row
	for rows.Next() {
		var r row
		var sha sql.NullString
		if err := rows.Scan(&r.id, &r.key, &r.mime, &r.location, &sha); err != nil {
			return 0, 0, []string{fmt.Sprintf("scan: %v", err)}
		}
		r.sha256 = sha.String
		todo = append(todo, r)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, []string{fmt.Sprintf("rows: %v", err)}
	}

	totalFound := len(todo)
	copied := 0
	var errs []string

	for _, r := range todo {
		src := s.BackendForLocation(r.location)
		rc, size, err := src.Get(ctx, r.key)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: read failed: %v", r.key, err))
			continue
		}

		// Buffer to verify SHA256 and provide Content-Length for S3 PutObject.
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: buffer failed: %v", r.key, err))
			continue
		}
		_ = size

		// Verify source integrity if SHA256 is stored.
		sum := sha256.Sum256(data)
		checksum := hex.EncodeToString(sum[:])
		if r.sha256 != "" && r.sha256 != checksum {
			errs = append(errs, fmt.Sprintf("%s: SHA256 mismatch (expected %s, got %s)", r.key, r.sha256, checksum))
			continue
		}

		// Write to target.
		if err := target.Put(ctx, r.key, bytes.NewReader(data), int64(len(data)), r.mime); err != nil {
			errs = append(errs, fmt.Sprintf("%s: write failed: %v", r.key, err))
			continue
		}

		// Verify target read-back.
		trc, _, err := target.Get(ctx, r.key)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: verify read failed: %v", r.key, err))
			continue
		}
		tData, err := io.ReadAll(trc)
		trc.Close()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: verify read failed: %v", r.key, err))
			continue
		}
		tSum := sha256.Sum256(tData)
		tChecksum := hex.EncodeToString(tSum[:])
		if tChecksum != checksum {
			errs = append(errs, fmt.Sprintf("%s: target SHA256 mismatch after write", r.key))
			target.Delete(ctx, r.key) // remove corrupt copy
			continue
		}

		// Update DB row.
		if _, err := s.db.ExecContext(ctx, `
			UPDATE attachments SET storage_location = ?, content_sha256 = ? WHERE id = ?`,
			target.Name(), checksum, r.id); err != nil {
			errs = append(errs, fmt.Sprintf("%s: DB update failed: %v", r.key, err))
			continue
		}
		copied++
	}
	return totalFound, copied, errs
}

// CleanupSource deletes files from the source backend for attachments that have
// been migrated to target (storage_location == target.Name()). Does not touch DB rows.
func (s *AttachmentStore) CleanupSource(ctx context.Context, source, target storage.Backend) (int, []string) {
	rows, err := s.db.Query(`
		SELECT stored_filename FROM attachments WHERE storage_location = ?`, target.Name())
	if err != nil {
		return 0, []string{fmt.Sprintf("query: %v", err)}
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return 0, []string{fmt.Sprintf("scan: %v", err)}
		}
		keys = append(keys, k)
	}

	removed := 0
	var errs []string
	for _, k := range keys {
		if err := source.Delete(ctx, k); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", k, err))
			continue
		}
		removed++
	}
	return removed, errs
}

// AttachmentForBackup carries the columns needed to compose a backup ZIP entry.
type AttachmentForBackup struct {
	ID              int64
	StoredFilename  string
	MimeType        string
	SizeBytes       int64
	StorageLocation string
	ContentSHA256   string
}

// EnumerateForBackup returns every attachment row, regardless of backend.
// Callers iterate the slice to read each file from its origin backend and
// stream it into the backup archive.
func (s *AttachmentStore) EnumerateForBackup(ctx context.Context) ([]AttachmentForBackup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stored_filename, mime_type, size_bytes, storage_location, content_sha256
		FROM attachments
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("enumerate attachments: %w", err)
	}
	defer rows.Close()

	var out []AttachmentForBackup
	for rows.Next() {
		var a AttachmentForBackup
		var sha sql.NullString
		if err := rows.Scan(&a.ID, &a.StoredFilename, &a.MimeType, &a.SizeBytes, &a.StorageLocation, &sha); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		a.ContentSHA256 = sha.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// BackfillSHA256 persists a freshly computed SHA256 for an attachment that did
// not have one yet (typical of local-backend uploads predating the column).
func (s *AttachmentStore) BackfillSHA256(ctx context.Context, attachmentID int64, sha256Hex string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE attachments SET content_sha256 = ? WHERE id = ?`,
		sha256Hex, attachmentID)
	if err != nil {
		return fmt.Errorf("backfill sha256: %w", err)
	}
	return nil
}

// AttachmentRef is the minimal info needed by restore to write a single
// attachment back to the active storage backend.
type AttachmentRef struct {
	ID              int64
	StoredFilename  string
	StorageLocation string
	MimeType        string
}

// LookupBySHA256 returns every attachment row whose content_sha256 matches
// the given hex digest. Used by restore to fan out a single ZIP entry to
// every storage filename that points at that content (deduplication).
//
// Uses idx_attachments_content_sha256 created in migrate.go.
func (s *AttachmentStore) LookupBySHA256(ctx context.Context, sha256Hex string) ([]AttachmentRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stored_filename, storage_location, mime_type
		FROM attachments
		WHERE content_sha256 = ?`, sha256Hex)
	if err != nil {
		return nil, fmt.Errorf("lookup by sha256: %w", err)
	}
	defer rows.Close()
	var out []AttachmentRef
	for rows.Next() {
		var r AttachmentRef
		if err := rows.Scan(&r.ID, &r.StoredFilename, &r.StorageLocation, &r.MimeType); err != nil {
			return nil, fmt.Errorf("scan ref: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ErrTooLarge is returned when an uploaded file exceeds the size limit.
var ErrTooLarge = errors.New("file too large")

// readWithLimit reads r up to maxBytes. Returns ErrTooLarge if exceeded.
func readWithLimit(r io.Reader, maxBytes int64) ([]byte, int64, error) {
	buf, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}
	if int64(len(buf)) > maxBytes {
		return nil, 0, ErrTooLarge
	}
	return buf, int64(len(buf)), nil
}
