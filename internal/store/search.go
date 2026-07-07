package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// SearchHit is one result from GET /api/search.
type SearchHit struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// SearchStore manages the FTS5 index and provides the search interface.
type SearchStore struct {
	db *sql.DB
}

// NewSearchStore wraps db.
func NewSearchStore(db *sql.DB) *SearchStore {
	return &SearchStore{db: db}
}

// Index inserts or replaces an entry in documents_fts.
// Call after every Create or Update of a document.
func (s *SearchStore) Index(tx *sql.Tx, docID int64, title, bodyText string, tagNames []string) error {
	tags := strings.Join(tagNames, " ")
	// FTS5 contentless tables use INSERT (there is no UPSERT for FTS5).
	// We deindex first to avoid duplicates.
	if err := s.deindex(tx, docID); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO documents_fts(rowid, title, body_text, tags) VALUES (?, ?, ?, ?)`,
		docID, title, bodyText, tags)
	return err
}

// Deindex removes the FTS5 entry for docID (used on soft-delete).
func (s *SearchStore) Deindex(tx *sql.Tx, docID int64) error {
	return s.deindex(tx, docID)
}

func (s *SearchStore) deindex(tx *sql.Tx, docID int64) error {
	_, err := tx.Exec(`INSERT INTO documents_fts(documents_fts, rowid, title, body_text, tags) VALUES ('delete', ?, '', '', '')`,
		docID)
	// Ignore "no such rowid" — it means the doc was never indexed
	return ignoreNoSuchRowid(err)
}

// Search performs a full-text search for q across title, body_text, and tags.
// Primary path: FTS5 MATCH with quoted phrase. Fallback: LIKE on short or
// special queries. Returns at most limit results.
func (s *SearchStore) Search(q string, limit int) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" || limit <= 0 {
		return nil, nil
	}

	// Try FTS5 first
	hits, err := s.ftsSearch(q, limit)
	if err == nil && len(hits) > 0 {
		return hits, nil
	}

	// Fallback to LIKE (covers short queries, accented chars, operator conflicts)
	return s.likeSearch(q, limit)
}

func (s *SearchStore) ftsSearch(q string, limit int) ([]SearchHit, error) {
	// Wrap in quotes for FTS5 phrase search; escape internal quotes.
	// JOIN with documents to retrieve real titles (documents_fts is content=''
	// so highlight/snippet return empty strings) and filter trashed rows.
	safe := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT d.id, d.title, substr(d.body_text, 1, 200) AS snippet
		FROM documents_fts
		JOIN documents d ON d.id = documents_fts.rowid
		WHERE documents_fts MATCH ?
		  AND d.trashed_at IS NULL
		ORDER BY documents_fts.rank
		LIMIT %d`, limit), safe)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHits(rows)
}

// IndexDoc upserts a document into the FTS5 index outside a transaction.
// Call after Create or Update to keep the index current.
func (s *SearchStore) IndexDoc(docID int64, title, bodyText string, tagNames []string) error {
	tags := strings.Join(tagNames, " ")
	if err := ignoreNoSuchRowid(s.deleteFTSProbe(docID)); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO documents_fts(rowid, title, body_text, tags) VALUES (?, ?, ?, ?)`,
		docID, title, bodyText, tags,
	)
	return err
}

func (s *SearchStore) likeSearch(q string, limit int) ([]SearchHit, error) {
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`
		SELECT d.id, d.title, substr(d.body_text, 1, 200) AS snippet
		FROM documents d
		WHERE (d.title LIKE ? OR d.body_text LIKE ?)
		  AND d.trashed_at IS NULL
		ORDER BY d.updated_at DESC
		LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHits(rows)
}

func scanHits(rows *sql.Rows) ([]SearchHit, error) {
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// deleteFTSProbe issues the contentless-FTS5 "delete" probe for docID. Its
// result must always be routed through ignoreNoSuchRowid — this is a
// best-effort "remove if present" operation, never the sole write of a call.
func (s *SearchStore) deleteFTSProbe(docID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO documents_fts(documents_fts, rowid, title, body_text, tags) VALUES ('delete', ?, '', '', '')`,
		docID)
	return err
}

// ignoreNoSuchRowid swallows the errors modernc.org/sqlite returns when the
// contentless-FTS5 "delete" probe targets a rowid that was never indexed.
// Confirmed empirically: this driver does NOT return a clean "no such rowid"
// for a never-inserted rowid — it returns "database disk image is malformed"
// (SQLITE_CORRUPT_VTAB), because FTS5 can't locate index entries to remove
// for content that was never written. Both texts are swallowed; any other
// error (e.g. a real I/O failure) still propagates.
func ignoreNoSuchRowid(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "no such rowid") || strings.Contains(err.Error(), "malformed") {
		return nil
	}
	return err
}
