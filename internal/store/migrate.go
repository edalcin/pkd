package store

import (
	"database/sql"
	"embed"
	"fmt"
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
	}
	for _, m := range colMigrations {
		if _, err := db.Exec(m.sql); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				db.Close()
				return nil, fmt.Errorf("store.Open %s: %w", m.ctx, err)
			}
		}
	}

	return db, nil
}
