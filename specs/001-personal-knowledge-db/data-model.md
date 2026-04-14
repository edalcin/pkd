# Phase 1 Data Model: Personal Knowledge Database (PKD)

**Feature**: `001-personal-knowledge-db`
**Date**: 2026-04-14
**Storage**: SQLite 3.40+ (single file at `$PKD_DB_PATH`), accessed through `modernc.org/sqlite`.

This document captures every persistent entity, its fields, relationships, validation rules, and state transitions. It is the authoritative source for `internal/store/schema.sql` and the Go types in `internal/model/`.

> **Conventions**
> - All timestamps are stored as ISO-8601 UTC strings (`TEXT`) to make backups human-readable.
> - All IDs are autoincrementing `INTEGER PRIMARY KEY` unless otherwise noted. Share tokens are the one exception.
> - Foreign keys are enforced (`PRAGMA foreign_keys = ON` at every connection open).
> - Deletes are cascading only where the parent-child relationship is strictly ownership-based (e.g., `document_tags` → `documents`).

---

## 1. Entity-relationship overview

```
┌──────────────┐       ┌────────────────┐       ┌─────────┐
│  documents   │ 1───* │ document_tags  │ *───1 │  tags   │
└──────┬───────┘       └────────────────┘       └─────────┘
       │ 1
       │
       │ *
┌──────▼────────┐       ┌──────────────┐
│  attachments  │       │ share_links  │
└───────────────┘       └──────┬───────┘
                               │ *
                               │
                               │ 1
                        ┌──────▼───────┐
                        │  documents   │   (same table as above)
                        └──────────────┘

┌──────────────────┐       ┌───────────────┐
│ documents_fts    │       │ login_attempts│   (ephemeral / in-memory, not persisted)
│ (FTS5 virtual)   │       └───────────────┘
└──────────────────┘
```

Sessions and login-attempt throttling state are **in-memory only** — they are documented here for completeness but do not appear in the SQL schema.

---

## 2. `documents`

Primary entity. One row per note. Folders are documents with children — there is no separate folder table.

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `id` | `INTEGER` | `PRIMARY KEY AUTOINCREMENT` | |
| `parent_id` | `INTEGER` | `NULL`, `FOREIGN KEY (parent_id) REFERENCES documents(id) ON DELETE RESTRICT` | `NULL` = root document. Restricted delete because deletions go through trash, not cascade. |
| `title` | `TEXT` | `NOT NULL`, length 1–500 | |
| `body_html` | `TEXT` | `NOT NULL DEFAULT ''` | Sanitized rich HTML emitted by CKEditor 5. Max 2 MB enforced in application layer. |
| `body_text` | `TEXT` | `NOT NULL DEFAULT ''` | Plain-text projection of `body_html`, computed on save. Used by FTS5 index. |
| `icon` | `TEXT` | `NULL` | Identifier of an icon in the shipped library (e.g., `lucide/book`). `NULL` → default icon. |
| `position` | `INTEGER` | `NOT NULL DEFAULT 0` | Sort order among siblings. Stable integer gaps (100, 200, 300…) to allow cheap inserts. |
| `version` | `INTEGER` | `NOT NULL DEFAULT 1` | **Optimistic-concurrency token.** Incremented on every successful save. See FR-010a. |
| `trashed_at` | `TEXT` | `NULL` | `NULL` = active. Non-NULL = in Trash; value is the ISO-8601 UTC timestamp of trashing. |
| `original_parent_id` | `INTEGER` | `NULL`, `FOREIGN KEY (original_parent_id) REFERENCES documents(id) ON DELETE SET NULL` | Where to restore to when un-trashed. `NULL` means "restore to root". |
| `created_at` | `TEXT` | `NOT NULL` | ISO-8601 UTC. Used by calendar view. |
| `updated_at` | `TEXT` | `NOT NULL` | ISO-8601 UTC. |

**Indexes**:
- `idx_documents_parent_id_position` on `(parent_id, position)` — drives tree rendering.
- `idx_documents_trashed_at` on `(trashed_at)` — speeds up the "exclude trash" filter used everywhere except the Trash view.
- `idx_documents_created_at` on `(created_at)` — drives calendar view.

**Validation rules (application layer)**:
- A move MUST reject `parent_id ∈ descendants(id)` (prevents circular moves; FR-006).
- A move MUST reject pointing at a trashed parent.
- `title` is trimmed of leading/trailing whitespace before validation.
- `body_html` is sanitized by the bluemonday editor policy before storage.
- Non-trashed documents MUST NOT have a non-trashed ancestor chain that includes any trashed ancestor — trashing a parent trashes its whole subtree atomically (see State transitions below).

**State transitions**:

```
             ┌──────────────────────────────── restore ────────────────────────────┐
             │                                                                      │
             ▼                                                                      │
   ┌──────────────────┐       trash(confirm)        ┌────────────────────┐          │
   │   Active         │ ──────────────────────────▶ │   Trashed           │ ─────────┘
   │ (trashed_at NULL)│                             │ (trashed_at SET)    │
   └──────────────────┘                             └────────┬───────────┘
                                                             │
                                                             │ emptyTrash / permanentDelete(id)
                                                             ▼
                                                      (row removed,
                                                       attachments deleted,
                                                       share_links deleted)
```

- **trash** operation: moves the document and its entire subtree from `Active` to `Trashed`. For the root of the operation, `original_parent_id := current parent_id`; for descendants, `original_parent_id` is left unchanged (they'll follow their original parent back out). Active share links are revoked at trash time (status → `revoked`).
- **restore** operation: sets `trashed_at = NULL` and moves the document back under `original_parent_id` (or to root if that parent is itself still trashed or no longer exists).
- **emptyTrash** / **permanentDelete**: deletes the row, cascades removal of `document_tags` rows, deletes `attachments` rows (and their files on disk), and deletes associated `share_links`.

Trash is **indefinite** (clarification Q2 → C): no automatic purge, ever.

---

## 3. `tags`

Hashtag dictionary. Stores each tag once; documents link via a join table so rename/merge is cheap.

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `id` | `INTEGER` | `PRIMARY KEY AUTOINCREMENT` | |
| `name` | `TEXT` | `NOT NULL UNIQUE COLLATE NOCASE` | Normalized: lowercase ASCII, no leading `#`, `[a-z0-9_\-]+`, length 1–64. |
| `created_at` | `TEXT` | `NOT NULL` | |

**Validation rules**:
- On tag creation/rename, `name` is normalized server-side; leading `#` is stripped; disallowed characters cause a 400.
- Uniqueness is case-insensitive via `COLLATE NOCASE` (also the column's sort order).

**Operations**:
- **Rename** (`#a` → `#b`): if `#b` does not exist, simply `UPDATE tags SET name = 'b' WHERE name = 'a'`.
- **Merge** (`#a` → existing `#b`): inside a transaction,
  1. `UPDATE document_tags SET tag_id = <b.id> WHERE tag_id = <a.id> AND document_id NOT IN (SELECT document_id FROM document_tags WHERE tag_id = <b.id>)`
  2. `DELETE FROM document_tags WHERE tag_id = <a.id>`  *(handles rows where both tags were present on the same doc)*
  3. `DELETE FROM tags WHERE id = <a.id>`.

---

## 4. `document_tags`

Many-to-many join between documents and tags.

| Column | Type | Constraints |
|---|---|---|
| `document_id` | `INTEGER` | `NOT NULL`, `FOREIGN KEY ... ON DELETE CASCADE` |
| `tag_id` | `INTEGER` | `NOT NULL`, `FOREIGN KEY ... ON DELETE CASCADE` |
| `PRIMARY KEY` | | `(document_id, tag_id)` |

**Indexes**:
- Primary key above already covers `(document_id, tag_id)`.
- `idx_document_tags_tag_id` on `(tag_id)` — drives "list all documents carrying tag X".

---

## 5. `attachments`

File metadata. The actual bytes live on the host-mounted attachments volume.

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `id` | `INTEGER` | `PRIMARY KEY AUTOINCREMENT` | |
| `document_id` | `INTEGER` | `NOT NULL`, `FOREIGN KEY ... ON DELETE RESTRICT` | Deletions go through the document delete path which also removes attachment files from disk. |
| `original_filename` | `TEXT` | `NOT NULL`, length 1–255 | User-supplied, used for display and download. Stored as-is; never used to construct a filesystem path. |
| `stored_filename` | `TEXT` | `NOT NULL UNIQUE` | App-generated random name (32 bytes base64url). Combined with two-level shard path. |
| `size_bytes` | `INTEGER` | `NOT NULL CHECK (size_bytes >= 0)` | |
| `mime_type` | `TEXT` | `NOT NULL` | Detected via `http.DetectContentType`, not trusted from client. |
| `created_at` | `TEXT` | `NOT NULL` | |

**On-disk path derivation**:
```
$PKD_ATTACHMENTS_PATH / stored_filename[0:2] / stored_filename[2:4] / stored_filename
```

**Validation rules**:
- `stored_filename` MUST be generated server-side from `crypto/rand`. Client-supplied names are ignored for the on-disk layout.
- Every write path through `filepath.Clean` + prefix assertion against `$PKD_ATTACHMENTS_PATH` (defense against path traversal, FR-044).
- If the attachments volume is unwritable, the upload fails before any DB row is inserted (FR-028).

---

## 6. `share_links`

Public read-only share tokens for documents. Stored as hashes, not plaintext (research §15).

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `id` | `INTEGER` | `PRIMARY KEY AUTOINCREMENT` | |
| `document_id` | `INTEGER` | `NOT NULL`, `FOREIGN KEY ... ON DELETE CASCADE` | |
| `token_hash` | `BLOB` | `NOT NULL UNIQUE` | SHA-256 of the 32-byte random token. Plaintext token is shown to the owner exactly once at creation time, never persisted. |
| `status` | `TEXT` | `NOT NULL CHECK (status IN ('active','revoked'))` | |
| `created_at` | `TEXT` | `NOT NULL` | |
| `revoked_at` | `TEXT` | `NULL` | Set when status flips to `revoked`. |

**Indexes**:
- `idx_share_links_token_hash` on `(token_hash)` — single indexed lookup per public request.
- `idx_share_links_document_id` on `(document_id)` — owner lists active links for a document.

**State transitions**:

```
┌──────────┐   revoke   ┌──────────┐
│ active   │ ─────────▶ │ revoked  │
└──────────┘            └──────────┘
    │                        │
    │ document permanently   │ document permanently
    │ deleted                │ deleted
    ▼                        ▼
  (row removed via ON DELETE CASCADE)
```

- Revocation is irreversible for a given token. A new share on the same document produces a fresh token.
- Trashing a document automatically revokes all its active share links.
- Restoring a trashed document does NOT re-activate its previously revoked share links (the owner must generate a new share if they want the doc public again).

---

## 7. `documents_fts` (FTS5 virtual table)

Full-text index supporting the universal search (FR-020..FR-022).

```sql
CREATE VIRTUAL TABLE documents_fts USING fts5(
  title,
  body_text,
  tags,                -- space-joined tag names for this document
  content='',          -- contentless table — we write directly, no sync shadow copy
  tokenize = 'unicode61 remove_diacritics 2'
);
```

**Maintenance**: application-level, not trigger-based. Every write path (`INSERT`, `UPDATE`, move to/from trash, tag change) explicitly updates `documents_fts`. Triggers were considered but rejected because the tag join would require cross-table triggers that become brittle; application-level maintenance is clearer and testable.

**Exclusions**: trashed documents are NOT in the FTS index — trashing a document issues a `DELETE FROM documents_fts WHERE rowid = ?`; restoring reinserts it.

**Query strategy**:
- Query `q` is split into tokens; each token becomes `q*` for prefix match.
- The FTS `MATCH` query is: `title:tok1* body_text:tok1* tags:tok1* OR title:tok2* body_text:tok2* tags:tok2* ...` with `bm25()` for ranking.
- For queries shorter than 3 chars or containing characters FTS5 tokenizer strips, fall back to `WHERE title LIKE '%q%' OR body_text LIKE '%q%'` against the main `documents` table (still indexed by `idx_documents_trashed_at` to prune trash).

---

## 8. In-memory state (not persisted)

These are documented for completeness — they live in the Go process, not in SQLite.

### 8.1 Sessions

```go
type Session struct {
    ID        string    // 32-byte crypto/rand, base64url
    CreatedAt time.Time
    LastSeen  time.Time // used for idle-timeout expiry
}
```
Stored in a `map[string]*Session` guarded by `sync.RWMutex`. Expired entries (LastSeen older than `PKD_SESSION_IDLE_MINUTES`) are lazily evicted on lookup and swept every 5 minutes.

### 8.2 Login-attempt throttler

```go
type AttemptState struct {
    Count       int
    FirstFailed time.Time
    LockedUntil time.Time // zero = not locked
}
```
Stored in a `sync.Map` keyed by source IP. On successful login or lockout expiry, the entry is cleared. Not persisted: a container restart is equivalent to a counter reset, which is acceptable given the threat model.

---

## 9. Initial schema SQL (authoritative)

This is the exact DDL that `internal/store/schema.sql` will embed. Migrations beyond v1 will be additive files (e.g., `schema_002.sql`) applied in lexical order.

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS documents (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_id           INTEGER REFERENCES documents(id) ON DELETE RESTRICT,
  title               TEXT NOT NULL,
  body_html           TEXT NOT NULL DEFAULT '',
  body_text           TEXT NOT NULL DEFAULT '',
  icon                TEXT,
  position            INTEGER NOT NULL DEFAULT 0,
  version             INTEGER NOT NULL DEFAULT 1,
  trashed_at          TEXT,
  original_parent_id  INTEGER REFERENCES documents(id) ON DELETE SET NULL,
  created_at          TEXT NOT NULL,
  updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_parent_id_position ON documents(parent_id, position);
CREATE INDEX IF NOT EXISTS idx_documents_trashed_at         ON documents(trashed_at);
CREATE INDEX IF NOT EXISTS idx_documents_created_at         ON documents(created_at);

CREATE TABLE IF NOT EXISTS tags (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS document_tags (
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  tag_id      INTEGER NOT NULL REFERENCES tags(id)      ON DELETE CASCADE,
  PRIMARY KEY (document_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_document_tags_tag_id ON document_tags(tag_id);

CREATE TABLE IF NOT EXISTS attachments (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  document_id       INTEGER NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
  original_filename TEXT NOT NULL,
  stored_filename   TEXT NOT NULL UNIQUE,
  size_bytes        INTEGER NOT NULL CHECK (size_bytes >= 0),
  mime_type         TEXT NOT NULL,
  created_at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attachments_document_id ON attachments(document_id);

CREATE TABLE IF NOT EXISTS share_links (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  token_hash  BLOB NOT NULL UNIQUE,
  status      TEXT NOT NULL CHECK (status IN ('active','revoked')),
  created_at  TEXT NOT NULL,
  revoked_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_share_links_document_id ON share_links(document_id);

CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
  title,
  body_text,
  tags,
  content='',
  tokenize = 'unicode61 remove_diacritics 2'
);
```

**Notes on pragmas**:
- `foreign_keys = ON` is mandatory; SQLite leaves it off by default.
- `journal_mode = WAL` enables concurrent readers during writes — important for the "backup while editing" requirement (SC-004).
- `synchronous = NORMAL` is the recommended WAL setting: safe against crashes, orders of magnitude faster than `FULL`.

---

## 10. Data volume & retention

- **Expected row counts at steady state**: up to ~20 k `documents`, ~2 k `tags`, ~50 k `document_tags`, ~5 k `attachments`, ~100 `share_links`.
- **Retention**: nothing is purged automatically. Trash is indefinite (clarification Q2 → C). Backups are manual only (Q3 → A).
- **Backup file format**: identical to the live SQLite file (produced via `VACUUM INTO`). A backup is self-describing and can be restored in-place.

---

## 11. Cross-references to spec requirements

| Requirement | Realized by |
|---|---|
| FR-005 (CRUD tree) | `documents` table + parent_id |
| FR-006 (no circular moves) | Application-layer descendant check |
| FR-007 (subtree preservation) | Move only updates root `parent_id`; descendants untouched |
| FR-008 (trash indefinite) | `trashed_at`, `original_parent_id` |
| FR-010a (optimistic concurrency) | `version` column + check on save |
| FR-011..FR-014 (rich editor body + images) | `body_html`, `body_text`, bluemonday policy |
| FR-015..FR-017 (hashtags) | `tags`, `document_tags`, rename/merge operation |
| FR-018..FR-019 (icons) | `documents.icon` |
| FR-020..FR-022 (search) | `documents_fts` + LIKE fallback |
| FR-023..FR-024 (calendar) | `documents.created_at` + index |
| FR-025..FR-028 (attachments external) | `attachments` table + sharded on-disk layout |
| FR-029..FR-032 (share links) | `share_links` with hashed tokens |
| FR-033..FR-037 (admin) | Operations on all tables above; no schema additions |
| FR-041 (storage boundary) | Single file at `$PKD_DB_PATH` + files under `$PKD_ATTACHMENTS_PATH` |
| FR-042..FR-045 (security cross-cutting) | Sanitization before write, hashed share tokens, path checks on attachments, parameterized queries everywhere |
