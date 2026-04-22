package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/model"
)

// URLStore provides CRUD for document_urls.
type URLStore struct {
	db *sql.DB
}

// NewURLStore wraps db.
func NewURLStore(db *sql.DB) *URLStore {
	return &URLStore{db: db}
}

// ListByDocument returns all URLs for the given document.
func (s *URLStore) ListByDocument(docID int64) ([]*model.DocumentURL, error) {
	rows, err := s.db.Query(`
		SELECT id, document_id, url, title, created_at
		FROM document_urls WHERE document_id = ? ORDER BY created_at`, docID)
	if err != nil {
		return nil, fmt.Errorf("urls.List: %w", err)
	}
	defer rows.Close()
	var out []*model.DocumentURL
	for rows.Next() {
		var u model.DocumentURL
		var ts string
		if err := rows.Scan(&u.ID, &u.DocumentID, &u.URL, &u.Title, &ts); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, &u)
	}
	if out == nil {
		out = []*model.DocumentURL{}
	}
	return out, rows.Err()
}

// Create inserts a new URL for the document.
func (s *URLStore) Create(docID int64, rawURL, title string) (*model.DocumentURL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("url is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(
		`INSERT INTO document_urls (document_id, url, title, created_at) VALUES (?, ?, ?, ?)`,
		docID, rawURL, strings.TrimSpace(title), now)
	if err != nil {
		return nil, fmt.Errorf("urls.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return &model.DocumentURL{
		ID:         id,
		DocumentID: docID,
		URL:        rawURL,
		Title:      strings.TrimSpace(title),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// Update changes the url and title of an existing DocumentURL.
func (s *URLStore) Update(id int64, rawURL, title string) (*model.DocumentURL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("url is required")
	}
	title = strings.TrimSpace(title)
	res, err := s.db.Exec(`UPDATE document_urls SET url = ?, title = ? WHERE id = ?`, rawURL, title, id)
	if err != nil {
		return nil, fmt.Errorf("urls.Update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	var u model.DocumentURL
	var ts string
	if err := s.db.QueryRow(
		`SELECT id, document_id, url, title, created_at FROM document_urls WHERE id = ?`, id,
	).Scan(&u.ID, &u.DocumentID, &u.URL, &u.Title, &ts); err != nil {
		return nil, fmt.Errorf("urls.Update fetch: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &u, nil
}

// Delete removes a URL by its ID.
func (s *URLStore) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM document_urls WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("urls.Delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAll returns all URLs across all documents (used by admin validation).
func (s *URLStore) ListAll() ([]*model.DocumentURL, error) {
	rows, err := s.db.Query(`
		SELECT id, document_id, url, title, created_at
		FROM document_urls ORDER BY document_id, created_at`)
	if err != nil {
		return nil, fmt.Errorf("urls.ListAll: %w", err)
	}
	defer rows.Close()
	var out []*model.DocumentURL
	for rows.Next() {
		var u model.DocumentURL
		var ts string
		if err := rows.Scan(&u.ID, &u.DocumentID, &u.URL, &u.Title, &ts); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, &u)
	}
	if out == nil {
		out = []*model.DocumentURL{}
	}
	return out, rows.Err()
}
