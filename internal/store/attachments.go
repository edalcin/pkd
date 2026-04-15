package store

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
)

// AttachmentStore persists attachment metadata and manages file storage.
type AttachmentStore struct {
	db              *sql.DB
	attachmentsPath string
}

// NewAttachmentStore wraps db and an on-disk attachments directory.
func NewAttachmentStore(db *sql.DB, attachmentsPath string) *AttachmentStore {
	return &AttachmentStore{db: db, attachmentsPath: attachmentsPath}
}

// CreateFile writes body to a sharded path under attachmentsPath and inserts
// the metadata row. If the stream exceeds maxBytes it is rejected.
// Returns the created Attachment with a populated URL.
func (s *AttachmentStore) CreateFile(docID int64, origName, mimeType string, body io.Reader, maxBytes int64) (*model.Attachment, error) {
	// Generate a random stored filename and shard path: ab/cd/<random>
	token := security.NewToken(16) // 16 bytes → 22 chars base64url
	shardDir := filepath.Join(s.attachmentsPath, token[:2], token[2:4])
	if err := os.MkdirAll(shardDir, 0750); err != nil {
		return nil, fmt.Errorf("mkdir shard: %w", err)
	}
	storedFilename := filepath.Join(token[:2], token[2:4], token)
	fullPath := filepath.Join(s.attachmentsPath, storedFilename)

	f, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}

	// Limit reads to maxBytes + 1 to detect oversized uploads
	written, err := io.Copy(f, io.LimitReader(body, maxBytes+1))
	f.Close()
	if err != nil {
		os.Remove(fullPath)
		return nil, fmt.Errorf("write file: %w", err)
	}
	if written > maxBytes {
		os.Remove(fullPath)
		return nil, ErrTooLarge
	}

	att := &model.Attachment{
		DocumentID:     docID,
		OriginalName:   origName,
		StoredFilename: storedFilename,
		MimeType:       mimeType,
		SizeBytes:      written,
	}

	err = WithTx(s.db, func(tx *sql.Tx) error {
		res, err := tx.Exec(`
			INSERT INTO attachments (document_id, original_name, stored_filename, mime_type, size_bytes, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			att.DocumentID, att.OriginalName, att.StoredFilename, att.MimeType, att.SizeBytes,
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
		os.Remove(fullPath)
		return nil, err
	}

	return att, nil
}

// GetByID returns the attachment metadata. Returns ErrNotFound if missing.
func (s *AttachmentStore) GetByID(id int64) (*model.Attachment, error) {
	var att model.Attachment
	var createdStr string
	err := s.db.QueryRow(`
		SELECT id, document_id, original_name, stored_filename, mime_type, size_bytes, created_at
		FROM attachments WHERE id = ?`, id).Scan(
		&att.ID, &att.DocumentID, &att.OriginalName, &att.StoredFilename,
		&att.MimeType, &att.SizeBytes, &createdStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	att.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	att.URL = fmt.Sprintf("/api/attachments/%d", att.ID)
	return &att, nil
}

// Delete removes the file on disk and the metadata row.
func (s *AttachmentStore) Delete(id int64) error {
	att, err := s.GetByID(id)
	if err != nil {
		return err
	}
	fullPath, err := security.SafeAttachmentPath(s.attachmentsPath, att.StoredFilename)
	if err != nil {
		return err
	}
	os.Remove(fullPath) // best-effort; proceed even if file is gone
	_, err = s.db.Exec(`DELETE FROM attachments WHERE id = ?`, id)
	return err
}

// ListByDocument returns all attachments for a document.
func (s *AttachmentStore) ListByDocument(docID int64) ([]*model.Attachment, error) {
	rows, err := s.db.Query(`
		SELECT id, document_id, original_name, stored_filename, mime_type, size_bytes, created_at
		FROM attachments WHERE document_id = ? ORDER BY created_at`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var atts []*model.Attachment
	for rows.Next() {
		var att model.Attachment
		var createdStr string
		if err := rows.Scan(&att.ID, &att.DocumentID, &att.OriginalName, &att.StoredFilename, &att.MimeType, &att.SizeBytes, &createdStr); err != nil {
			return nil, err
		}
		att.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		att.URL = fmt.Sprintf("/api/attachments/%d", att.ID)
		atts = append(atts, &att)
	}
	return atts, rows.Err()
}

// ListOrphanedStoredFiles returns stored filenames that have no matching row.
// Used by the admin cleanup handler.
func (s *AttachmentStore) ListOrphanedStoredFiles() ([]string, error) {
	// Walk the attachments directory and compare to database rows
	rows, err := s.db.Query(`SELECT stored_filename FROM attachments`)
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
		known[filepath.Clean(filepath.Join(s.attachmentsPath, sf))] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var orphans []string
	err = filepath.Walk(s.attachmentsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if _, ok := known[filepath.Clean(path)]; !ok {
			orphans = append(orphans, path)
		}
		return nil
	})
	return orphans, err
}

// ErrTooLarge is returned when an uploaded file exceeds the size limit.
var ErrTooLarge = errors.New("file too large")

// FullPath resolves the on-disk path for an attachment, performing path
// traversal validation. Returns ErrPathTraversal on invalid filenames.
func (s *AttachmentStore) FullPath(att *model.Attachment) (string, error) {
	return security.SafeAttachmentPath(s.attachmentsPath, att.StoredFilename)
}
