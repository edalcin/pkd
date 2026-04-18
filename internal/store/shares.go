package store

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
)

// ShareStore manages share-link tokens.
type ShareStore struct {
	db *sql.DB
}

// NewShareStore wraps db.
func NewShareStore(db *sql.DB) *ShareStore {
	return &ShareStore{db: db}
}

// Create generates a share link for docID. Returns the plaintext token (shown
// once) and the ShareLink record containing the row ID needed for revocation.
// The token is NOT stored — only its SHA-256 hash is persisted.
func (s *ShareStore) Create(docID int64) (plaintext string, share *model.ShareLink, err error) {
	plaintext = security.NewToken(32) // 32 bytes → 43 chars base64url
	hash := security.HashSHA256(plaintext)

	now := time.Now().UTC()
	var id int64
	err = WithTx(s.db, func(tx *sql.Tx) error {
		res, err := tx.Exec(`
			INSERT INTO share_links (document_id, token_hash, created_at)
			VALUES (?, ?, ?)`,
			docID, hash, now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	share = &model.ShareLink{
		ID:         id,
		DocumentID: docID,
		TokenHash:  hash,
		CreatedAt:  now,
	}
	return plaintext, share, nil
}

// LookupByToken finds an active (non-revoked) share link by plaintext token.
// Returns ErrNotFound if the token does not match any active link.
func (s *ShareStore) LookupByToken(plaintext string) (*model.ShareLink, error) {
	hash := security.HashSHA256(plaintext)

	rows, err := s.db.Query(`
		SELECT id, document_id, token_hash, created_at
		FROM share_links
		WHERE revoked_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sl model.ShareLink
		var tokenHash []byte
		var createdStr string
		if err := rows.Scan(&sl.ID, &sl.DocumentID, &tokenHash, &createdStr); err != nil {
			return nil, err
		}
		if subtle.ConstantTimeCompare(hash, tokenHash) == 1 {
			sl.TokenHash = tokenHash
			sl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
			return &sl, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, ErrNotFound
}

// Revoke marks a share link as revoked by setting revoked_at.
func (s *ShareStore) Revoke(shareID int64) error {
	res, err := s.db.Exec(`
		UPDATE share_links SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339Nano), shareID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByDocument returns all share links for a document.
func (s *ShareStore) ListByDocument(docID int64) ([]*model.ShareLink, error) {
	rows, err := s.db.Query(`
		SELECT id, document_id, token_hash, created_at, revoked_at
		FROM share_links WHERE document_id = ? ORDER BY created_at`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []*model.ShareLink
	for rows.Next() {
		var sl model.ShareLink
		var createdStr string
		var revokedStr sql.NullString
		if err := rows.Scan(&sl.ID, &sl.DocumentID, &sl.TokenHash, &createdStr, &revokedStr); err != nil {
			return nil, err
		}
		sl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if revokedStr.Valid {
			t, _ := time.Parse(time.RFC3339Nano, revokedStr.String)
			sl.RevokedAt = &t
		}
		links = append(links, &sl)
	}
	return links, rows.Err()
}

// ListAllActive returns all non-revoked share links joined with their document title,
// ordered newest first. Used by the admin panel.
func (s *ShareStore) ListAllActive() ([]*model.ShareWithDoc, error) {
	rows, err := s.db.Query(`
		SELECT sl.id, sl.document_id, d.title, sl.created_at
		FROM share_links sl
		JOIN documents d ON d.id = sl.document_id
		WHERE sl.revoked_at IS NULL AND d.trashed_at IS NULL
		ORDER BY sl.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []*model.ShareWithDoc
	for rows.Next() {
		var sl model.ShareWithDoc
		var createdStr string
		if err := rows.Scan(&sl.ID, &sl.DocumentID, &sl.DocumentTitle, &createdStr); err != nil {
			return nil, err
		}
		sl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		links = append(links, &sl)
	}
	return links, rows.Err()
}

// ErrRevoked is returned when a share link exists but has been revoked.
var ErrRevoked = errors.New("share link revoked")
