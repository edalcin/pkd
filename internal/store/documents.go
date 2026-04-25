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

// iconLeaf is the default icon for documents with no children.
const iconLeaf = "bx-dock-top"

// iconFolder is the default icon for documents that have children.
const iconFolder = "bxs-folder"

// DocumentStore provides all document persistence operations.
type DocumentStore struct {
	db *sql.DB
}

// NewDocumentStore wraps db.
func NewDocumentStore(db *sql.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

// syncParentIcon updates the parent's icon based on whether it has remaining
// non-trashed children. Only modifies icons that were auto-assigned (empty,
// iconLeaf, or iconFolder) — user-set custom icons are never touched.
func syncParentIcon(tx *sql.Tx, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	var nullIcon sql.NullString
	if err := tx.QueryRow(`SELECT icon FROM documents WHERE id = ?`, *parentID).Scan(&nullIcon); err != nil {
		return err
	}
	currentIcon := nullIcon.String
	if currentIcon != "" && currentIcon != iconLeaf && currentIcon != iconFolder {
		return nil // user-set icon, leave it alone
	}
	var childCount int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM documents WHERE parent_id = ? AND trashed_at IS NULL`, *parentID,
	).Scan(&childCount); err != nil {
		return err
	}
	newIcon := iconLeaf
	if childCount > 0 {
		newIcon = iconFolder
	}
	if newIcon == currentIcon {
		return nil
	}
	_, err := tx.Exec(
		`UPDATE documents SET icon = ?, updated_at = `+nowISO+` WHERE id = ?`,
		newIcon, *parentID)
	return err
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
			INSERT INTO documents (parent_id, title, icon, position, created_at, updated_at)
			VALUES (?, ?, ?, ?, `+nowISO+`, `+nowISO+`)`,
			parentArg, title, iconLeaf, pos)
		if err != nil {
			return fmt.Errorf("insert: %w", err)
		}
		id, _ := res.LastInsertId()
		doc.ID = id
		if err := scanDocFromTx(tx, id, &doc); err != nil {
			return err
		}
		return syncParentIcon(tx, parentID)
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
	return s.UpdateAndSync(id, clientVersion, title, bodyHTML, bodyText, icon, nil)
}

// UpdateAndSync is like Update but additionally runs syncFn inside the same
// transaction after the document row is saved. syncFn receives the open *sql.Tx
// so it can atomically update derived data (e.g. document_links). If syncFn is
// nil the behaviour is identical to Update.
func (s *DocumentStore) UpdateAndSync(id int64, clientVersion int64, title, bodyHTML, bodyText, icon string, syncFn func(*sql.Tx) error) (*model.Document, error) {
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
		if err := scanDocFromTx(tx, id, &doc); err != nil {
			return err
		}
		if syncFn != nil {
			return syncFn(tx)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// ToggleFavorite flips the is_favorite flag on a document and returns the updated doc.
func (s *DocumentStore) ToggleFavorite(id int64) (*model.Document, error) {
	res, err := s.db.Exec(
		`UPDATE documents SET is_favorite = 1 - is_favorite WHERE id = ? AND trashed_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.GetByID(id)
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
		if _, err := tx.Exec(`
			UPDATE documents
			SET trashed_at = `+nowISO+`, original_parent_id = parent_id, parent_id = NULL
			WHERE id = ?`, id); err != nil {
			return err
		}
		if parentID.Valid {
			v := parentID.Int64
			return syncParentIcon(tx, &v)
		}
		return nil
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

// Reorder moves a document to newParentID and places it before beforeID within
// that parent's children. beforeID == nil appends the document at the end.
// Also updates the position field for every affected sibling.
func (s *DocumentStore) Reorder(id int64, newParentID *int64, beforeID *int64) error {
	if newParentID != nil && *newParentID == id {
		return ErrCircularMove
	}
	return WithTx(s.db, func(tx *sql.Tx) error {
		var oldParentID sql.NullInt64
		if err := tx.QueryRow(`SELECT parent_id FROM documents WHERE id = ? AND trashed_at IS NULL`, id).Scan(&oldParentID); err != nil {
			return err
		}
		if newParentID != nil {
			if err := checkCircular(tx, id, *newParentID); err != nil {
				return err
			}
		}
		siblings, err := querySiblingIDs(tx, newParentID, id)
		if err != nil {
			return err
		}
		insertAt := len(siblings)
		if beforeID != nil {
			for i, sid := range siblings {
				if sid == *beforeID {
					insertAt = i
					break
				}
			}
		}
		ordered := make([]int64, 0, len(siblings)+1)
		ordered = append(ordered, siblings[:insertAt]...)
		ordered = append(ordered, id)
		ordered = append(ordered, siblings[insertAt:]...)

		var parentArg interface{}
		if newParentID != nil {
			parentArg = *newParentID
		}
		if _, err := tx.Exec(
			`UPDATE documents SET parent_id = ?, updated_at = `+nowISO+` WHERE id = ? AND trashed_at IS NULL`,
			parentArg, id); err != nil {
			return err
		}
		if err := reassignPositions(tx, ordered); err != nil {
			return err
		}
		if oldParentID.Valid {
			v := oldParentID.Int64
			if err := syncParentIcon(tx, &v); err != nil {
				return err
			}
		}
		return syncParentIcon(tx, newParentID)
	})
}

// SortAll re-orders every level of the tree by the given criterion.
// Accepted values for by: "alpha" (title A-Z), "created" (oldest first), "updated" (newest first).
func (s *DocumentStore) SortAll(by string) error {
	var orderClause string
	switch by {
	case "alpha":
		orderClause = "title COLLATE NOCASE ASC, id ASC"
	case "created":
		orderClause = "created_at ASC, id ASC"
	case "updated":
		orderClause = "updated_at DESC, id ASC"
	default:
		return fmt.Errorf("unknown sort criterion: %s", by)
	}
	return WithTx(s.db, func(tx *sql.Tx) error {
		prows, err := tx.Query(`SELECT DISTINCT parent_id FROM documents WHERE trashed_at IS NULL`)
		if err != nil {
			return err
		}
		type group struct{ id *int64 }
		var groups []group
		for prows.Next() {
			var pid sql.NullInt64
			if err := prows.Scan(&pid); err != nil {
				prows.Close()
				return err
			}
			if pid.Valid {
				v := pid.Int64
				groups = append(groups, group{&v})
			} else {
				groups = append(groups, group{nil})
			}
		}
		prows.Close()
		if err := prows.Err(); err != nil {
			return err
		}
		for _, g := range groups {
			var crows *sql.Rows
			if g.id == nil {
				crows, err = tx.Query(
					`SELECT id FROM documents WHERE parent_id IS NULL AND trashed_at IS NULL ORDER BY `+orderClause)
			} else {
				crows, err = tx.Query(
					`SELECT id FROM documents WHERE parent_id = ? AND trashed_at IS NULL ORDER BY `+orderClause, *g.id)
			}
			if err != nil {
				return err
			}
			var ids []int64
			for crows.Next() {
				var cid int64
				if err := crows.Scan(&cid); err != nil {
					crows.Close()
					return err
				}
				ids = append(ids, cid)
			}
			crows.Close()
			if err := crows.Err(); err != nil {
				return err
			}
			if err := reassignPositions(tx, ids); err != nil {
				return err
			}
		}
		return nil
	})
}

func querySiblingIDs(tx *sql.Tx, parentID *int64, excludeID int64) ([]int64, error) {
	var rows *sql.Rows
	var err error
	if parentID == nil {
		rows, err = tx.Query(
			`SELECT id FROM documents WHERE parent_id IS NULL AND trashed_at IS NULL AND id != ? ORDER BY position ASC, id ASC`,
			excludeID)
	} else {
		rows, err = tx.Query(
			`SELECT id FROM documents WHERE parent_id = ? AND trashed_at IS NULL AND id != ? ORDER BY position ASC, id ASC`,
			*parentID, excludeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func reassignPositions(tx *sql.Tx, ids []int64) error {
	for pos, id := range ids {
		if _, err := tx.Exec(`UPDATE documents SET position = ? WHERE id = ?`, pos, id); err != nil {
			return err
		}
	}
	return nil
}

// Move changes a document's parent. Rejects self-move and circular moves.
// newParentID == nil moves the document to root level.
func (s *DocumentStore) Move(id int64, newParentID *int64) error {
	if newParentID != nil && *newParentID == id {
		return ErrCircularMove
	}
	return WithTx(s.db, func(tx *sql.Tx) error {
		var oldParentID sql.NullInt64
		if err := tx.QueryRow(`SELECT parent_id FROM documents WHERE id = ? AND trashed_at IS NULL`, id).Scan(&oldParentID); err != nil {
			return err
		}
		if newParentID != nil {
			if err := checkCircular(tx, id, *newParentID); err != nil {
				return err
			}
		}
		var parentArg interface{}
		if newParentID != nil {
			parentArg = *newParentID
		}
		if _, err := tx.Exec(`
			UPDATE documents
			SET parent_id = ?, updated_at = `+nowISO+`
			WHERE id = ? AND trashed_at IS NULL`, parentArg, id); err != nil {
			return err
		}
		if oldParentID.Valid {
			v := oldParentID.Int64
			if err := syncParentIcon(tx, &v); err != nil {
				return err
			}
		}
		return syncParentIcon(tx, newParentID)
	})
}

// ListTree returns all non-trashed documents. If tagFilter is non-empty, only
// documents that carry ALL specified tags are included. If favoriteOnly is true,
// only documents with is_favorite = 1 are returned. If q is non-empty, only
// documents whose title, body_text, or associated external link titles match are
// returned. The returned slice is a flat list; callers build the tree structure.
func (s *DocumentStore) ListTree(tagFilter []string, favoriteOnly bool, q string) ([]*model.Document, error) {
	if q != "" {
		return s.listByQuery(tagFilter, favoriteOnly, q)
	}
	if len(tagFilter) > 0 {
		return s.listByTags(tagFilter, favoriteOnly)
	}
	extra := ""
	if favoriteOnly {
		extra = " AND is_favorite = 1"
	}
	rows, err := s.db.Query(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, is_favorite, created_at, updated_at
		FROM documents
		WHERE trashed_at IS NULL` + extra + `
		ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocRows(rows)
}

// listByQuery returns documents matching q across title, body_text, and external
// link titles. Optional tagFilter and favoriteOnly further narrow the results.
func (s *DocumentStore) listByQuery(tagFilter []string, favoriteOnly bool, q string) ([]*model.Document, error) {
	pattern := "%" + q + "%"
	cond := `d.trashed_at IS NULL AND (d.title LIKE ? OR d.body_text LIKE ? OR EXISTS (
		SELECT 1 FROM document_urls u WHERE u.document_id = d.id AND u.title LIKE ?
	))`
	args := []interface{}{pattern, pattern, pattern}
	if favoriteOnly {
		cond += " AND d.is_favorite = 1"
	}
	if len(tagFilter) > 0 {
		ph := ""
		for i, t := range tagFilter {
			if i > 0 {
				ph += ","
			}
			ph += "?"
			args = append(args, t)
		}
		cond += fmt.Sprintf(` AND d.id IN (
			SELECT dt.document_id FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
			WHERE t.name IN (%s) GROUP BY dt.document_id HAVING COUNT(DISTINCT t.id) = %d
		)`, ph, len(tagFilter))
	}
	rows, err := s.db.Query(`
		SELECT d.id, d.parent_id, d.title, d.body_html, d.body_text, d.icon, d.position, d.version, d.is_favorite, d.created_at, d.updated_at
		FROM documents d
		WHERE `+cond+`
		ORDER BY d.position ASC, d.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocRows(rows)
}

// ListTrash returns all trashed documents with their trashed_at timestamp.
func (s *DocumentStore) ListTrash() ([]*model.Document, error) {
	rows, err := s.db.Query(`
		SELECT id, parent_id, title, icon, trashed_at
		FROM documents
		WHERE trashed_at IS NOT NULL
		ORDER BY trashed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []*model.Document
	for rows.Next() {
		var doc model.Document
		var parentID sql.NullInt64
		var icon sql.NullString
		var trashedStr string
		if err := rows.Scan(&doc.ID, &parentID, &doc.Title, &icon, &trashedStr); err != nil {
			return nil, err
		}
		if parentID.Valid {
			doc.ParentID = &parentID.Int64
		}
		doc.Icon = icon.String
		t, _ := time.Parse(time.RFC3339Nano, trashedStr)
		doc.TrashedAt = &t
		docs = append(docs, &doc)
	}
	return docs, rows.Err()
}

// PermanentDelete hard-deletes a trashed document. Use EmptyTrash for bulk.
// Non-trashed children are moved to root to satisfy the FK RESTRICT constraint.
func (s *DocumentStore) PermanentDelete(id int64) error {
	return WithTx(s.db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE documents SET parent_id = NULL WHERE parent_id = ?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM documents WHERE id = ? AND trashed_at IS NOT NULL`, id)
		return err
	})
}

// EmptyTrash hard-deletes all trashed documents.
// Non-trashed children of trashed parents are moved to root first (FK RESTRICT).
func (s *DocumentStore) EmptyTrash() error {
	return WithTx(s.db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			UPDATE documents SET parent_id = NULL
			WHERE trashed_at IS NULL
			  AND parent_id IN (SELECT id FROM documents WHERE trashed_at IS NOT NULL)`); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM documents WHERE trashed_at IS NOT NULL`)
		return err
	})
}

// ListChildren returns all non-trashed direct children of parentID, ordered by position then id.
func (s *DocumentStore) ListChildren(parentID int64) ([]*model.Document, error) {
	rows, err := s.db.Query(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, is_favorite, created_at, updated_at
		FROM documents
		WHERE parent_id = ? AND trashed_at IS NULL
		ORDER BY position ASC, id ASC`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocRows(rows)
}

// listByTags returns documents that carry ALL tags in the filter (AND semantics).
func (s *DocumentStore) listByTags(tags []string, favoriteOnly bool) ([]*model.Document, error) {
	// Build: WHERE id IN (SELECT document_id FROM document_tags JOIN tags ... GROUP BY HAVING count = len(tags))
	extra := ""
	if favoriteOnly {
		extra = " AND d.is_favorite = 1"
	}
	query := `
		SELECT d.id, d.parent_id, d.title, d.body_html, d.body_text, d.icon, d.position, d.version, d.is_favorite, d.created_at, d.updated_at
		FROM documents d
		WHERE d.trashed_at IS NULL` + extra + `
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
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, is_favorite, created_at, updated_at
		FROM documents WHERE id = ? AND trashed_at IS NULL`, id)
	if err := scanDocRow(row, doc); err != nil {
		return err
	}
	doc.Tags, _ = queryTagNames(db, id)
	doc.AttachmentIDs, _ = queryAttachmentIDs(db, id)
	return nil
}

func scanDocFromTx(tx *sql.Tx, id int64, doc *model.Document) error {
	row := tx.QueryRow(`
		SELECT id, parent_id, title, body_html, body_text, icon, position, version, is_favorite, created_at, updated_at
		FROM documents WHERE id = ?`, id)
	if err := scanDocRow(row, doc); err != nil {
		return err
	}
	doc.Tags, _ = queryTagNamesTx(tx, id)
	doc.AttachmentIDs, _ = queryAttachmentIDsTx(tx, id)
	return nil
}

func scanDocRow(row *sql.Row, doc *model.Document) error {
	var parentID sql.NullInt64
	var bodyHTML, bodyText, icon sql.NullString
	var createdStr, updatedStr string
	err := row.Scan(
		&doc.ID, &parentID, &doc.Title,
		&bodyHTML, &bodyText, &icon,
		&doc.Position, &doc.Version,
		&doc.IsFavorite,
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

func queryTagNames(db *sql.DB, docID int64) ([]string, error) {
	rows, err := db.Query(`
		SELECT t.name FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		WHERE dt.document_id = ? ORDER BY t.name`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	return names, rows.Err()
}

func queryTagNamesTx(tx *sql.Tx, docID int64) ([]string, error) {
	rows, err := tx.Query(`
		SELECT t.name FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		WHERE dt.document_id = ? ORDER BY t.name`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	return names, rows.Err()
}

func queryAttachmentIDs(db *sql.DB, docID int64) ([]int64, error) {
	rows, err := db.Query(`SELECT id FROM attachments WHERE document_id = ? ORDER BY created_at`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func queryAttachmentIDsTx(tx *sql.Tx, docID int64) ([]int64, error) {
	rows, err := tx.Query(`SELECT id FROM attachments WHERE document_id = ? ORDER BY created_at`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
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
			&doc.IsFavorite,
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
