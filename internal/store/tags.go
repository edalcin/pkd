package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/edalcin/pkd/internal/model"
)

// TagStore manages tag persistence and normalization.
type TagStore struct {
	db *sql.DB
}

// NewTagStore wraps db.
func NewTagStore(db *sql.DB) *TagStore {
	return &TagStore{db: db}
}

// NormalizeName lowercases the tag, strips a leading '#', and trims space.
// Returns an empty string if the result is empty after normalization.
func NormalizeName(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "#")
	s = strings.ToLower(s)
	// Collapse internal whitespace
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	return s
}

// SetDocumentTags replaces all tags on a document with the provided set.
// Normalizes names, upserts missing tags, diffs join rows.
func (s *TagStore) SetDocumentTags(docID int64, rawNames []string) error {
	return WithTx(s.db, func(tx *sql.Tx) error {
		// Normalize and deduplicate
		seen := make(map[string]struct{})
		var names []string
		for _, raw := range rawNames {
			n := NormalizeName(raw)
			if n == "" {
				continue
			}
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			names = append(names, n)
		}

		// Upsert each tag
		tagIDs := make([]int64, 0, len(names))
		for _, name := range names {
			id, err := upsertTag(tx, name)
			if err != nil {
				return err
			}
			tagIDs = append(tagIDs, id)
		}

		// Replace join rows: delete all then insert desired set
		if _, err := tx.Exec(`DELETE FROM document_tags WHERE document_id = ?`, docID); err != nil {
			return err
		}
		for _, tagID := range tagIDs {
			if _, err := tx.Exec(`INSERT INTO document_tags (document_id, tag_id) VALUES (?, ?)`, docID, tagID); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListWithCounts returns all tags ordered by name with document counts.
func (s *TagStore) ListWithCounts() ([]*model.TagWithCount, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.name, COUNT(dt.document_id) as cnt
		FROM tags t
		LEFT JOIN document_tags dt ON dt.tag_id = t.id
		LEFT JOIN documents d ON d.id = dt.document_id AND d.trashed_at IS NULL
		GROUP BY t.id
		ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []*model.TagWithCount
	for rows.Next() {
		var t model.TagWithCount
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, err
		}
		tags = append(tags, &t)
	}
	return tags, rows.Err()
}

// ListForDocument returns the tag names for a given document.
func (s *TagStore) ListForDocument(docID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT t.name FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		WHERE dt.document_id = ?
		ORDER BY t.name`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// RenameOrMerge renames oldName to newName. If newName already exists, all
// join rows from oldName are re-homed to newName's tag and oldName is deleted
// (merge). If newName does not exist, the tag row is updated in place.
func (s *TagStore) RenameOrMerge(oldName, newName string) error {
	oldNorm := NormalizeName(oldName)
	newNorm := NormalizeName(newName)
	if oldNorm == "" || newNorm == "" {
		return errors.New("tag names must not be empty")
	}
	if oldNorm == newNorm {
		return nil // nothing to do
	}

	return WithTx(s.db, func(tx *sql.Tx) error {
		var oldID int64
		if err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, oldNorm).Scan(&oldID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("tag %q not found", oldNorm)
			}
			return err
		}

		var newID int64
		err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, newNorm).Scan(&newID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if errors.Is(err, sql.ErrNoRows) {
			// newName does not exist — rename in place
			_, err = tx.Exec(`UPDATE tags SET name = ? WHERE id = ?`, newNorm, oldID)
			return err
		}

		// newName exists — merge: move join rows to newID, delete oldID
		// Avoid duplicates: delete conflicting pairs first
		_, err = tx.Exec(`
			DELETE FROM document_tags
			WHERE tag_id = ? AND document_id IN (
				SELECT document_id FROM document_tags WHERE tag_id = ?
			)`, oldID, newID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE document_tags SET tag_id = ? WHERE tag_id = ?`, newID, oldID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM tags WHERE id = ?`, oldID)
		return err
	})
}

// upsertTag inserts a tag by name if it doesn't exist and returns its ID.
func upsertTag(tx *sql.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO tags (name, created_at) VALUES (?, ?)`,
		name, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
