package store

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// Open opens the SQLite database at dbPath, applies the required PRAGMAs, and
// runs the embedded schema.sql migration. The migration is idempotent (all DDL
// uses IF NOT EXISTS), so it is safe to call on an already-initialised database.
//
// The "file::memory:?cache=shared&mode=memory" URI is accepted for in-process tests.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store.Open: %w", err)
	}

	// Limit to one writer connection. WAL mode allows concurrent readers but
	// SQLite itself serialises writers, so a pool of 1 prevents "database is
	// locked" errors on busy_timeout without unnecessary contention.
	db.SetMaxOpenConns(1)

	// Connection-level PRAGMAs — must be applied BEFORE the schema transaction.
	// journal_mode and synchronous cannot be changed inside a transaction, so
	// they are executed separately here.
	//
	// For in-memory databases (":memory:" or "mode=memory") WAL is not
	// supported; we skip it silently so tests work without a file path.
	isMemory := strings.Contains(dbPath, ":memory:") || strings.Contains(dbPath, "mode=memory")

	if !isMemory {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("store.Open pragma journal_mode: %w", err)
		}
		if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("store.Open pragma synchronous: %w", err)
		}
	}

	connectionPragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range connectionPragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("store.Open pragma %q: %w", p, err)
		}
	}

	// Apply the schema inside a transaction (safe because it contains no PRAGMAs).
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open read schema: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open begin: %w", err)
	}
	if _, err := tx.Exec(string(schema)); err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("store.Open exec schema: %w", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open commit: %w", err)
	}

	// Idempotent column migrations — ignore "duplicate column name" errors.
	colMigrations := []struct {
		sql string
		ctx string
	}{
		{`ALTER TABLE documents ADD COLUMN is_favorite INTEGER NOT NULL DEFAULT 0`, "alter documents is_favorite"},
		{`ALTER TABLE document_links ADD COLUMN manual INTEGER NOT NULL DEFAULT 0`, "alter document_links"},
		{`ALTER TABLE tags ADD COLUMN color TEXT NOT NULL DEFAULT ''`, "alter tags color"},
		{`ALTER TABLE share_links ADD COLUMN token_plain TEXT NOT NULL DEFAULT ''`, "alter share_links token_plain"},
		{`ALTER TABLE share_links ADD COLUMN is_auto INTEGER NOT NULL DEFAULT 0`, "alter share_links is_auto"},
		{`ALTER TABLE documents ADD COLUMN assoc_year  INTEGER`, "alter documents assoc_year"},
		{`ALTER TABLE documents ADD COLUMN assoc_month INTEGER`, "alter documents assoc_month"},
		{`ALTER TABLE documents ADD COLUMN assoc_day   INTEGER`, "alter documents assoc_day"},
		{`ALTER TABLE documents ADD COLUMN locked      INTEGER NOT NULL DEFAULT 0`, "alter documents locked"},
		{`ALTER TABLE documents ADD COLUMN archived_at TEXT`, "alter documents archived_at"},
		{`ALTER TABLE attachments ADD COLUMN storage_location TEXT NOT NULL DEFAULT 'local'`, "alter attachments storage_location"},
		{`ALTER TABLE attachments ADD COLUMN content_sha256 TEXT`, "alter attachments content_sha256"},
	}
	for _, m := range colMigrations {
		if _, err := db.Exec(m.sql); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				db.Close()
				return nil, fmt.Errorf("store.Open %s: %w", m.ctx, err)
			}
		}
	}

	// Create index on archived_at after the column migration above ensures the
	// column exists on both fresh installs and upgraded databases.
	if _, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_documents_archived_at ON documents(archived_at) WHERE archived_at IS NOT NULL`,
	); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open idx_documents_archived_at: %w", err)
	}

	// Index on content_sha256 supports SHA256 lookups during attachment restore
	// (005-s3-attachments-backup).
	if _, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_attachments_content_sha256 ON attachments(content_sha256) WHERE content_sha256 IS NOT NULL`,
	); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open idx_attachments_content_sha256: %w", err)
	}

	// Settings table for key/value config (e.g. active storage backend).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open settings table: %w", err)
	}

	// Seed default active storage backend if not yet set.
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value, updated_at)
		VALUES ('attachments.backend', 'local', datetime('now'))`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open settings seed: %w", err)
	}

	// Seed default version retention limit if not yet set.
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value, updated_at)
		VALUES ('versions.max_per_doc', '50', datetime('now'))`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open settings seed versions: %w", err)
	}

	// Data migration: backfill associated date from created_at for existing documents.
	// Idempotent — only touches rows where assoc_year is still NULL.
	if _, err := db.Exec(`
		UPDATE documents
		SET
			assoc_year  = CAST(strftime('%Y', created_at) AS INTEGER),
			assoc_month = CAST(strftime('%m', created_at) AS INTEGER),
			assoc_day   = CAST(strftime('%d', created_at) AS INTEGER)
		WHERE assoc_year IS NULL
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open assoc_date backfill: %w", err)
	}

	// Data migration: assign default icons to existing documents that have none.
	// Documents with children get the folder icon; leaves get the leaf icon.
	// Runs at every startup but only touches rows with empty/null icon.
	if _, err := db.Exec(`
		UPDATE documents
		SET icon = CASE
			WHEN (SELECT COUNT(*) FROM documents c
			      WHERE c.parent_id = documents.id AND c.trashed_at IS NULL) > 0
			THEN 'bxs-folder'
			ELSE 'bx-dock-top'
		END
		WHERE trashed_at IS NULL AND (icon IS NULL OR icon = '')
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open icon data migration: %w", err)
	}

	// Warn about duplicate document titles and attempt to create a unique index.
	// If duplicates exist the index creation will be skipped; application-layer
	// checks in UpdateAndSync still prevent new duplicates from being introduced.
	{
		dupRows, qErr := db.Query(`
			SELECT title, COUNT(*) AS cnt
			FROM documents
			WHERE trashed_at IS NULL
			GROUP BY title COLLATE NOCASE
			HAVING cnt > 1`)
		if qErr == nil {
			var hasDups bool
			for dupRows.Next() {
				var t string
				var cnt int
				if dupRows.Scan(&t, &cnt) == nil {
					if !hasDups {
						log.Printf("WARNING: duplicate document titles found — unique index not applied until resolved:")
						hasDups = true
					}
					log.Printf("  title=%q  count=%d", t, cnt)
				}
			}
			dupRows.Close()
			if !hasDups {
				if _, idxErr := db.Exec(
					`CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_title_unique ON documents(title COLLATE NOCASE) WHERE trashed_at IS NULL`,
				); idxErr != nil && !strings.Contains(idxErr.Error(), "UNIQUE") {
					db.Close()
					return nil, fmt.Errorf("store.Open idx_documents_title_unique: %w", idxErr)
				}
			}
		}
	}

	// Rebuild the FTS5 search index from the documents table on every startup.
	// documents_fts uses content='' (contentless) so it must be maintained by the
	// application; delete-all + re-insert is idempotent and fast for personal-KB
	// sizes (<10k docs). The JOIN filter in ftsSearch handles trashed rows at query
	// time, but rebuilding here ensures the index reflects the current document set.
	if _, err := db.Exec(`INSERT INTO documents_fts(documents_fts) VALUES('delete-all')`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open fts5 delete-all: %w", err)
	}
	if _, err := db.Exec(`
		INSERT INTO documents_fts(rowid, title, body_text, tags)
		SELECT d.id, d.title, d.body_text,
		       COALESCE((
		           SELECT group_concat(t.name, ' ')
		           FROM document_tags dt
		           JOIN tags t ON t.id = dt.tag_id
		           WHERE dt.document_id = d.id
		       ), '')
		FROM documents d
		WHERE d.trashed_at IS NULL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open fts5 rebuild: %w", err)
	}

	return db, nil
}
