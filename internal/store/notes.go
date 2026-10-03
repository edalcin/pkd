package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/model"
)

// ErrNoteHierarchy is returned when an operation would give a Nota a parent
// or children. Notas live only in the Notas block, never in the normal tree.
var ErrNoteHierarchy = errors.New("notas não têm pai nem filhos")

// iconNote is the default icon of a new Nota (boxicons, Q16).
const iconNote = "bx-sticky-note"

func isNote(q rowQuerier, id int64) bool {
	var n int
	_ = q.QueryRow(`SELECT COUNT(*) FROM documents WHERE id = ? AND is_note = 1`, id).Scan(&n)
	return n > 0
}

func rejectNoteParent(tx *sql.Tx, parentID *int64) error {
	if parentID != nil && isNote(tx, *parentID) {
		return ErrNoteHierarchy
	}
	return nil
}

func rejectNoteMove(tx *sql.Tx, id int64, newParentID *int64) error {
	if isNote(tx, id) {
		return ErrNoteHierarchy
	}
	return rejectNoteParent(tx, newParentID)
}

func scanNoteFields(q rowQuerier, id int64, doc *model.Document) error {
	var n int
	if err := q.QueryRow(`SELECT is_note FROM documents WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	doc.IsNote = n == 1
	return nil
}

// CreateNote inserts a Nota. A non-empty key already used returns the
// existing Nota with created=false (client idempotency). createdAt/updatedAt,
// when non-nil, override the server clock — callers must only pass them for
// Bearer-authenticated requests (Q11: datas explícitas só via Bearer).
func (s *DocumentStore) CreateNote(title, key string, favorite bool, createdAt, updatedAt *time.Time) (doc *model.Document, created bool, err error) {
	var id int64
	err = WithTx(s.db, func(tx *sql.Tx) error {
		if key != "" {
			err := tx.QueryRow(`SELECT id FROM documents WHERE note_key = ? AND trashed_at IS NULL`, key).Scan(&id)
			if err == nil {
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var keyArg any
		if key != "" {
			keyArg = key
		}
		title = uniqueTitle(tx, title)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		createdStr, updatedStr := now, now
		if createdAt != nil {
			createdStr = createdAt.UTC().Format(time.RFC3339Nano)
		}
		if updatedAt != nil {
			updatedStr = updatedAt.UTC().Format(time.RFC3339Nano)
		}
		res, err := tx.Exec(`
			INSERT INTO documents (parent_id, title, icon, position, created_at, updated_at, is_favorite, is_note, note_key)
			VALUES (NULL, ?, ?, 0, ?, ?, ?, 1, NULLIF(?, ''))`,
			title, iconNote, createdStr, updatedStr, favorite, keyArg)
		if err != nil {
			return fmt.Errorf("insert note: %w", err)
		}
		id, _ = res.LastInsertId()
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	doc, err = s.GetByID(id)
	return doc, created, err
}

// SetUpdatedAt overwrites updated_at. CreateNote writes the body and tags in
// later steps that bump updated_at; this restores a Bearer-supplied value (Q11).
func (s *DocumentStore) SetUpdatedAt(id int64, t time.Time) error {
	_, err := s.db.Exec(`UPDATE documents SET updated_at = ? WHERE id = ?`, t.UTC().Format(time.RFC3339Nano), id)
	return err
}

// GetNote returns the active Nota with the given document id. Returns
// ErrNotFound if the document is missing, trashed, or not a Nota.
func (s *DocumentStore) GetNote(id int64) (*model.Document, error) {
	doc, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if !doc.IsNote {
		return nil, ErrNotFound
	}
	return doc, nil
}

// NoteListItem is one line of the Notas block (GET /api/notes). BodyHTML,
// Tags and UpdatedAt let the Android app cache every Nota from one request.
type NoteListItem struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	Icon       string   `json:"icon"`
	IsFavorite bool     `json:"is_favorite"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	BodyHTML   string   `json:"body_html"` // empty when the Nota is encrypted
	Tags       []string `json:"tags"`
}

// ListNotes returns non-trashed Notas for view ("active" default |
// "archived" | "all", same as ListTree): favorites first, then created_at
// descending (Q4), optionally filtered by tag (AND semantics) and/or
// favorites-only (Q5), same as the normal tree filters.
func (s *DocumentStore) ListNotes(view string, tagFilter []string, favoriteOnly bool) ([]NoteListItem, error) {
	favExtra := ""
	switch view {
	case "archived":
		favExtra = " AND archived_at IS NOT NULL"
	case "all":
	default:
		favExtra = " AND archived_at IS NULL"
	}
	if favoriteOnly {
		favExtra += " AND is_favorite = 1"
	}
	// ponytail: tags joined with char(31) (unit separator), a byte tag names never hold.
	query := `SELECT id, title, COALESCE(icon, ''), is_favorite, created_at, updated_at,
		CASE WHEN encrypted = 1 THEN '' ELSE COALESCE(body_html, '') END,
		COALESCE((SELECT GROUP_CONCAT(t.name, char(31)) FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
			WHERE dt.document_id = documents.id), '')
		FROM documents
		WHERE is_note = 1 AND trashed_at IS NULL` + favExtra
	var args []any
	if len(tagFilter) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(tagFilter)), ",")
		query += fmt.Sprintf(` AND id IN (
			SELECT dt.document_id FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
			WHERE t.name IN (%s) GROUP BY dt.document_id HAVING COUNT(DISTINCT t.id) = %d
		)`, ph, len(tagFilter))
		args = make([]any, len(tagFilter))
		for i, t := range tagFilter {
			args[i] = t
		}
	}
	query += ` ORDER BY is_favorite DESC, created_at DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NoteListItem{}
	for rows.Next() {
		var n NoteListItem
		var isFav int
		var tags string
		if err := rows.Scan(&n.ID, &n.Title, &n.Icon, &isFav, &n.CreatedAt, &n.UpdatedAt, &n.BodyHTML, &tags); err != nil {
			return nil, err
		}
		n.IsFavorite = isFav == 1
		n.Tags = []string{}
		if tags != "" {
			n.Tags = strings.Split(tags, "\x1f")
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// NoteDocIDs returns the document IDs of all Notas, so search results (which
// mix Notas, Memórias and Documentos) can mark them.
func (s *DocumentStore) NoteDocIDs() (map[int64]bool, error) {
	rows, err := s.db.Query(`SELECT id FROM documents WHERE is_note = 1 AND trashed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// ConvertNoteToDocument turns a Nota into a normal Documento (Q3, Q18,
// one-way): clears is_note/note_key, restores the Documento default icon if
// it was still the Nota default, and places it at parentID/beforeID via the
// same logic as Reorder. Title, body, tags, attachments and favorite are
// unchanged.
func (s *DocumentStore) ConvertNoteToDocument(id int64, parentID, beforeID *int64) (*model.Document, error) {
	err := WithTx(s.db, func(tx *sql.Tx) error {
		if !isNote(tx, id) {
			return ErrNotFound
		}
		_, err := tx.Exec(`
			UPDATE documents
			SET is_note = 0, note_key = NULL,
			    icon = CASE WHEN icon = ? THEN ? ELSE icon END,
			    updated_at = `+nowISO+`
			WHERE id = ?`, iconNote, iconLeaf, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.Reorder(id, parentID, beforeID); err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

// ConvertNoteToMemory turns a Nota into a Memória (Q3, Q17, one-way): clears
// is_note/note_key, emits a fresh Memória ID (same retry-on-collision as
// CreateMemory) and sets the Data da Memória. Body, tags, attachments and
// favorite are unchanged. Caller must call d.Validate() first.
func (s *DocumentStore) ConvertNoteToMemory(id int64, d MemoryDate) (*model.Document, error) {
	err := WithTx(s.db, func(tx *sql.Tx) error {
		if !isNote(tx, id) {
			return ErrNotFound
		}
		for range 5 {
			mid, err := newMemoryID(d)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`
				UPDATE documents
				SET is_note = 0, note_key = NULL, icon = ?,
				    assoc_year = ?, assoc_month = ?, assoc_day = ?,
				    memory_id = ?, memory_hour = ?, memory_minute = ?, memory_period = NULLIF(?, ''),
				    updated_at = `+nowISO+`
				WHERE id = ?`,
				iconMemory, d.Year, d.Month, d.Day, mid, d.Hour, d.Minute, d.Period, id)
			if err != nil && strings.Contains(err.Error(), "documents.memory_id") {
				continue
			}
			return err
		}
		return errors.New("memory id collision: retries exhausted")
	})
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}
