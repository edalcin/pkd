-- PKD schema — applied once at startup via migrate.go (idempotent).
-- PRAGMAs are set programmatically in Open() before this runs.
-- All timestamps are ISO-8601 strings in UTC: YYYY-MM-DDTHH:MM:SS.sssZ

-- ---------------------------------------------------------------------------
-- documents
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS documents (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id           INTEGER REFERENCES documents(id) ON DELETE RESTRICT,
    title               TEXT    NOT NULL DEFAULT 'Untitled',
    body_html           TEXT    NOT NULL DEFAULT '',
    body_text           TEXT    NOT NULL DEFAULT '',
    icon                TEXT,
    position            INTEGER NOT NULL DEFAULT 0,
    version             INTEGER NOT NULL DEFAULT 1,
    is_favorite         INTEGER NOT NULL DEFAULT 0,
    locked              INTEGER NOT NULL DEFAULT 0,
    archived_at         TEXT,
    trashed_at          TEXT,
    original_parent_id  INTEGER REFERENCES documents(id) ON DELETE SET NULL,
    created_at          TEXT    NOT NULL,
    updated_at          TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_parent_id   ON documents(parent_id)   WHERE trashed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_documents_trashed_at  ON documents(trashed_at)  WHERE trashed_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_documents_archived_at ON documents(archived_at) WHERE archived_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_documents_updated_at  ON documents(updated_at);

-- ---------------------------------------------------------------------------
-- FTS5 full-text search (contentless — maintained at the application layer)
-- ---------------------------------------------------------------------------
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    title,
    body_text,
    tags,
    content='',
    tokenize='unicode61 remove_diacritics 2'
);

-- ---------------------------------------------------------------------------
-- tags
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    created_at TEXT    NOT NULL
);

-- ---------------------------------------------------------------------------
-- document_tags (join table)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS document_tags (
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags(id)      ON DELETE CASCADE,
    PRIMARY KEY (document_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_document_tags_tag_id ON document_tags(tag_id);

-- ---------------------------------------------------------------------------
-- attachments
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS attachments (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id      INTEGER NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    original_name    TEXT    NOT NULL,
    stored_filename  TEXT    NOT NULL UNIQUE,
    mime_type        TEXT    NOT NULL DEFAULT 'application/octet-stream',
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attachments_document_id ON attachments(document_id);

-- ---------------------------------------------------------------------------
-- document_links (bidirectional links between documents)
-- Simple directed edges: source → target. Backlinks derived via reverse query.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS document_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    target_id   INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at  TEXT    NOT NULL,
    UNIQUE(source_id, target_id)
);

CREATE INDEX IF NOT EXISTS idx_document_links_source ON document_links(source_id);
CREATE INDEX IF NOT EXISTS idx_document_links_target ON document_links(target_id);

-- ---------------------------------------------------------------------------
-- share_links
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS share_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    token_hash  BLOB    NOT NULL UNIQUE,
    created_at  TEXT    NOT NULL,
    revoked_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_share_links_document_id ON share_links(document_id);
CREATE INDEX IF NOT EXISTS idx_share_links_token_hash  ON share_links(token_hash);

-- ---------------------------------------------------------------------------
-- document_urls (external links associated with documents)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS document_urls (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    url         TEXT    NOT NULL,
    title       TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_document_urls_document_id ON document_urls(document_id);

-- ---------------------------------------------------------------------------
-- sessions (persistent across server restarts)
-- Timestamps stored as Unix epoch integers for compact storage.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT    PRIMARY KEY NOT NULL,
    ip           TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
);
