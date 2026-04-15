package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/edalcin/pkd/internal/model"
)

// ErrVersionConflict is returned by Update when the client's version does not
// match the version currently stored in the database.
var ErrVersionConflict = errors.New("version conflict")

// ErrNotFound is returned when a requested document does not exist or is
// not accessible (e.g. trashed when active is required).
var ErrNotFound = errors.New("not found")

// ErrCircularMove is returned when a Move would create a cycle in the tree.
var ErrCircularMove = errors.New("circular move: cannot move a node under its own descendant")

const nowISO = "strftime('%Y-%m-%dT%H:%M:%fZ','now')"

// DocumentStore provides all document persistence operations.
type DocumentStore struct {
	db *sql.DB
}

// NewDocumentStore wraps db.
func NewDocumentStore(db *sql.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

// Create inserts a new document and returns it with server-assigned fields.
func (s *DocumentStore) Create(parentID *int64, title string) (*model.Document, error) {
	var doc model.Document
	err := WithTx(s.db, func(tx *sql.Tx) error {
		// Determine next position among siblings
		var maxPos sql.NullInt64
		if parentID == nil {
			err := tx.QueryRow(`SELECT MAX(position) FROM documents WHERE parent_id IS NULL AND trashed_at IS NULL`).Scan(&maxPos)
			if err != nil {
				return fmt.Errorf("position query: %w", err)
			}
		} else {
			err := tx.QueryRow(`SELECT MAX(position) FROM documents WHERE parent_id = ? AND trashed_at IS NULL`, *parentID).Scan(&maxPos)
			if err != nil {
				return fmt.Errorf("position query: %w", err)
			}
		}
		pos := 0
		if maxPos.Valid {
			pos = int(maxPos.Int64) + 1
		}

		var parentArg interface{}
		if parentID != nil {
			parentArg = *parentID
		}

		res, err := tx.Exec(`
			INSERT INTO documents (parent_id, title, position, created_at, updated_at)
			VALUES (?, ?, ?, `+nowISO+`, `+nowISO+`)`,
			parentArg, title, pos)
		if err != nil {
			return fmt.Errorf("insert: %w", err)
		}
		id, _ := res.LastInsertId()
		doc.ID = id
		return scanDocFromTx(tx, id, &doc)
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetByID returns a document by ID. Returns ErrNotFound if missing or trashed.
func (s *DocumentStore) GetByID(id int64) (*model.Document, error) {
	var doc model.Document
	err := scanDoc(s.db, id, &doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &doc, err
}

// Update saves title, body_html, body_text, icon, and version-checks.
// If the stored version differs from clientVersion, returns ErrVersionConflict.
func (s *DocumentStore) Update(id int64, clientVersion int64, title, bodyHTML, bodyText, icon string) (*model.Document, error) {
	var doc model.Document
	err := WithTx(s.db, func(tx *sql.Tx) error {
		var storedVersion int64
		if err := tx.QueryRow(`SELECT version FROM documents WHERE id = ? AND trashed_at IS NULL`, id).Scan(&storedVersion); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("version check: %w", err)
		}
		if storedVersion != clientVersion {
			return ErrVersionConflict
		}
		_, err := tx.Exec(`
			UPDATE documents
			SET title = ?, body_html = ?, body_text = ?, icon = ?,
			    version = version + 1, updated_at = `+nowISO+`
			WHERE id = ? AND trashed_at IS NULL`,
			title, bodyHTML, bodyText, icon, id)
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		return scanDocFromTx(tx, id, &doc)
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// SoftDelete moves a document to the trash by setting trashed_at and saving
// original_parent_id. Its children are NOT trashed — the caller should check.
func (s *DocumentStore) SoftDelete(id int64) error {
	return WithTx(s.db, func(tx *sql.Tx) error {
		var parentID sql.NullInt64
		if err := tx.QueryRow(`SELECT parent_id FROM documents WHERE id = ? AND trashed_at IS NULL`, id).Scan(&parentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		_, err := tx.Exec(`
			UPDATE documents
			SET trashed_at = `+nowISO+`, original_parent_id = parent_id, parent_id = NULL
			WHERE id = ?`, id)
		return err
	})
}

// Restore moves a document from trash back to its original parent.
// If the original parent is itself trashed, the document is restored to root.
func (s *DocumentStore) Restore(id int64) error {
	return WithTx(s.db, func(tx *sql.Tx) error {
		var origParentID sql.NullInt64
		if err := tx.QueryRow(`SELECT original_parent_id FROM documents WHERE id = ? AND trashed_at IS NOT NULL`, id).Scan(&origParentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		// Verify original parent is still alive (not trashed)
		var parentArg interface{}
		if origParentID.Valid {
			var alive bool
			tx.QueryRow(`SELECT 1 FROM documents WHERE id = ? AND trashed_at IS NULL`, origParentID.Int64).Scan(&alive)
			if alive {
				parentArg = origParentID.Int64
			}
			// if not alive, parentArg stays nil → restore to root
		}
		_, err := tx.Exec(`
			UPDATE documents
			SET trashed_at = NULL, parent_id = ?, original_parent_id = NULL
			WHERE id = ?`, parentArg, id)
		return err
	})
}

// Move changes a document's parent. Rejects self-move and circular moves.
// newParentID == nil moves the document to root level.
func (s *DocumentStore) Move(id int64, newParentID *int64) error {
	if newParentID != nil && *newParentID == id {
		return ErrCircularMove
	}
	return WithTx(s.db, func(tx *sql.Tx) error {
		if newParentID != nil {
			// Verify newParent is not a descendant of id (circular check)
			if err := checkCircular(tx, id, *newParentID); err != nil {
				return err
			}
		}
		var parentArg interface{}
		if newParentID != nil {
			parentArg = *newParentID
		}
		_, err := tx.Exec(`
			UPDATE documents
			SET parent_id = ?, updated_at = `+nowISO+`
			WHERE id = ? AND trashed_at IS NULL`, parentArg, id)
		return err
	})
}

// ListTree returns all non-trashed documents. If tagFilter is non-empty, only
// documents that carry ALL specified tags are included.
// The returned slice is a flat list; callers build the tree structure.
func (s *DocumentStore) ListTree(tagFilter []string) ([]*model.Document, error) {
	if len(tagFilter) > 0 {
		return s.listByTags(tagFilter)
	}
	rows, err := s.db.Query(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, created_at, updated_at
		FROM documents
		WHERE trashed_at IS NULL
		ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocRows(rows)
}

// ListTrash returns all trashed documents.
func (s *DocumentStore) ListTrash() ([]*model.Document, error) {
	rows, err := s.db.Query(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, created_at, updated_at
		FROM documents
		WHERE trashed_at IS NOT NULL
		ORDER BY trashed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocRows(rows)
}

// PermanentDelete hard-deletes a trashed document. Use EmptyTrash for bulk.
func (s *DocumentStore) PermanentDelete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM documents WHERE id = ? AND trashed_at IS NOT NULL`, id)
	return err
}

// EmptyTrash hard-deletes all trashed documents.
func (s *DocumentStore) EmptyTrash() error {
	_, err := s.db.Exec(`DELETE FROM documents WHERE trashed_at IS NOT NULL`)
	return err
}

// listByTags returns documents that carry ALL tags in the filter (AND semantics).
func (s *DocumentStore) listByTags(tags []string) ([]*model.Document, error) {
	// Build: WHERE id IN (SELECT document_id FROM document_tags JOIN tags ... GROUP BY HAVING count = len(tags))
	query := `
		SELECT d.id, d.parent_id, d.title, d.body_html, d.body_text, d.icon, d.position, d.version, d.created_at, d.updated_at
		FROM documents d
		WHERE d.trashed_at IS NULL
		  AND d.id IN (
			SELECT dt.document_id
			FROM document_tags dt
			JOIN tags t ON t.id = dt.tag_id
			WHERE t.name IN (`
	args := make([]interface{}, len(tags))
	for i, t := range tags {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = t
	}
	query += fmt.Sprintf(`) GROUP BY dt.document_id HAVING COUNT(DISTINCT t.id) = %d)
		ORDER BY d.position ASC, d.id ASC`, len(tags))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocRows(rows)
}

// checkCircular returns ErrCircularMove if targetID is a descendant of sourceID.
// It walks ancestors of targetID upward until it finds sourceID or hits root.
func checkCircular(tx *sql.Tx, sourceID, targetID int64) error {
	current := targetID
	for {
		var parentID sql.NullInt64
		err := tx.QueryRow(`SELECT parent_id FROM documents WHERE id = ?`, current).Scan(&parentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil // target not found → not a descendant
			}
			return err
		}
		if !parentID.Valid {
			return nil // reached root without hitting source → safe
		}
		if parentID.Int64 == sourceID {
			return ErrCircularMove
		}
		current = parentID.Int64
	}
}

// ─── scan helpers ────────────────────────────────────────────────────────────

func scanDoc(db *sql.DB, id int64, doc *model.Document) error {
	row := db.QueryRow(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, created_at, updated_at
		FROM documents WHERE id = ? AND trashed_at IS NULL`, id)
	return scanDocRow(row, doc)
}

func scanDocFromTx(tx *sql.Tx, id int64, doc *model.Document) error {
	row := tx.QueryRow(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, created_at, updated_at
		FROM documents WHERE id = ?`, id)
	return scanDocRow(row, doc)
}

func scanDocRow(row *sql.Row, doc *model.Document) error {
	var parentID sql.NullInt64
	var bodyHTML, bodyText, icon sql.NullString
	var createdStr, updatedStr string
	err := row.Scan(
		&doc.ID, &parentID, &doc.Title,
		&bodyHTML, &bodyText, &icon,
		&doc.Position, &doc.Version,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return err
	}
	if parentID.Valid {
		doc.ParentID = &parentID.Int64
	}
	doc.BodyHTML = bodyHTML.String
	doc.BodyText = bodyText.String
	doc.Icon = icon.String
	doc.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	doc.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return nil
}

func scanDocRows(rows *sql.Rows) ([]*model.Document, error) {
	var docs []*model.Document
	for rows.Next() {
		var doc model.Document
		var parentID sql.NullInt64
		var bodyHTML, bodyText, icon sql.NullString
		var createdStr, updatedStr string
		if err := rows.Scan(
			&doc.ID, &parentID, &doc.Title,
			&bodyHTML, &bodyText, &icon,
			&doc.Position, &doc.Version,
			&createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		if parentID.Valid {
			doc.ParentID = &parentID.Int64
		}
		doc.BodyHTML = bodyHTML.String
		doc.BodyText = bodyText.String
		doc.Icon = icon.String
		doc.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		doc.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		docs = append(docs, &doc)
	}
	return docs, rows.Err()
}
