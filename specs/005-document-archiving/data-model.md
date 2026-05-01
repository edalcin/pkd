# Data Model: Document Archiving

**Branch**: `005-document-archiving` | **Date**: 2026-05-01

## Database Changes

### Migration: Add `archived_at` column

```sql
-- Run once against existing database
ALTER TABLE documents ADD COLUMN archived_at TEXT;

-- Index for efficient filtering by archive status
CREATE INDEX IF NOT EXISTS idx_documents_archived_at ON documents(archived_at);
```

### Updated `documents` table (full column list after migration)

| Column | Type | Nullable | Default | Notes |
|--------|------|----------|---------|-------|
| id | INTEGER | NO | autoincrement | PK |
| parent_id | INTEGER | YES | NULL | FK → documents(id) ON DELETE RESTRICT |
| title | TEXT | NO | 'Untitled' | |
| body_html | TEXT | NO | '' | |
| body_text | TEXT | NO | '' | Derived from HTML, used for search |
| icon | TEXT | YES | NULL | Boxicons class name |
| position | INTEGER | NO | 0 | Ordering within siblings |
| version | INTEGER | NO | 1 | Optimistic lock counter |
| is_favorite | INTEGER | NO | 0 | Boolean (0/1) |
| locked | INTEGER | NO | 0 | Boolean (0/1) |
| **archived_at** | **TEXT** | **YES** | **NULL** | **ISO-8601; NULL = active, non-null = archived** |
| trashed_at | TEXT | YES | NULL | ISO-8601; soft delete |
| original_parent_id | INTEGER | YES | NULL | Saved on trash for restore |
| created_at | TEXT | NO | | ISO-8601 |
| updated_at | TEXT | NO | | ISO-8601 |
| assoc_year | INTEGER | YES | NULL | User-editable associated year |
| assoc_month | INTEGER | YES | NULL | User-editable associated month |
| assoc_day | INTEGER | YES | NULL | User-editable associated day |

### Schema.sql update

Add to the `CREATE TABLE documents` statement:
```sql
archived_at TEXT,
```
(after the `locked` column, before `trashed_at`)

---

## Go Model Changes

### `internal/model/document.go`

```go
// Document struct — add field:
ArchivedAt *time.Time `json:"archived_at"`

// DocumentTreeNode struct — add field:
ArchivedAt *time.Time `json:"archived_at"`
// Convenience derived field for frontend:
Archived   bool       `json:"archived"`
```

The `Archived` bool is computed as `ArchivedAt != nil` when building tree nodes.

---

## Store Layer Changes

### New error sentinel — `internal/store/documents.go`

```go
var ErrArchived = errors.New("document is archived")
```

### New methods

```go
// Archive sets archived_at to the current UTC time.
// Returns ErrNotFound if document doesn't exist or is trashed.
// Returns ErrLocked if document is locked (FR-017).
// Returns ErrArchived if document is already archived (no-op guard).
func (s *DocumentStore) Archive(id int64) (*model.Document, error)

// Unarchive clears archived_at.
// Returns ErrNotFound if document doesn't exist or is trashed.
// Returns nil error if already active (idempotent).
func (s *DocumentStore) Unarchive(id int64) (*model.Document, error)
```

### Modified methods

**`Update()`** — add archived check before version check:
```go
// After fetching storedVersion and locked:
var archivedAt sql.NullString
row.Scan(&storedVersion, &locked, &archivedAt)
if archivedAt.Valid {
    return ErrArchived
}
if locked {
    return ErrLocked
}
```

**`ListTree(tagFilter []string, favoriteOnly bool, q string)`** — add `view` parameter:
```go
func (s *DocumentStore) ListTree(view string, tagFilter []string, favoriteOnly bool, q string) ([]*model.Document, error)
```

View values and their SQL implications:
- `"active"` (default): `WHERE archived_at IS NULL AND trashed_at IS NULL` — plus recursive CTE when no search/tag filter (see below)
- `"archived"`: `WHERE archived_at IS NOT NULL AND trashed_at IS NULL`
- `"all"`: `WHERE trashed_at IS NULL`

**Active tree recursive CTE** (used when view=active and no search/tag filter):
```sql
WITH RECURSIVE active_tree AS (
    -- Seed: root documents that are active
    SELECT id, parent_id, title, icon, position, version,
           is_favorite, locked, archived_at, created_at, updated_at,
           assoc_year, assoc_month, assoc_day
    FROM documents
    WHERE parent_id IS NULL
      AND archived_at IS NULL
      AND trashed_at IS NULL
    UNION ALL
    -- Recursive: children of active parents
    SELECT d.id, d.parent_id, d.title, d.icon, d.position, d.version,
           d.is_favorite, d.locked, d.archived_at, d.created_at, d.updated_at,
           d.assoc_year, d.assoc_month, d.assoc_day
    FROM documents d
    JOIN active_tree a ON d.parent_id = a.id
    WHERE d.archived_at IS NULL
      AND d.trashed_at IS NULL
)
SELECT * FROM active_tree
ORDER BY position ASC, id ASC
```

**Search query** (`listByQuery`) — add `archived_at` filter parameter:
- When called from tree with view=active: add `AND d.archived_at IS NULL`
- When called from tree with view=archived: add `AND d.archived_at IS NOT NULL`
- When called from tree with view=all, OR from search endpoint: no archived_at filter

### `buildTree()` behavior change

Current: orphaned nodes (parent not in result set) are promoted to root.

New behavior by view:
- `"active"` **without** search/tag filter: recursive CTE ensures no orphaned nodes exist — buildTree behavior unchanged (no orphans to promote).
- `"active"` **with** search/tag filter: orphaned nodes promoted to root (current behavior preserved, matches existing UX for filtered trees).
- `"archived"`: orphaned nodes promoted to root (same as search filter behavior).
- `"all"`: no filtering, no orphans — buildTree behavior unchanged.

---

## State Transition Diagram

```
          archive()         unarchive()
ACTIVE ─────────────► ARCHIVED ◄────────────── (any view)
  │                      │
  │ softDelete()          │ softDelete()
  ▼                      ▼
TRASHED               TRASHED
  │                      │
  │ restore()             │ restore()
  ▼                      ▼
ACTIVE                 ACTIVE  ← restored docs are always unarchived on restore
```

Note: `restore()` (untrash) should also clear `archived_at` to avoid a restored document being in a hidden archived state.

---

## Frontend Data Contracts

### `DocumentTreeNode` JSON shape (updated)

```json
{
  "id": 42,
  "parent_id": null,
  "title": "My Document",
  "icon": "bx-file-blank",
  "position": 0,
  "version": 3,
  "is_favorite": false,
  "locked": false,
  "archived": false,
  "archived_at": null,
  "tags": [],
  "children": []
}
```

### `Document` JSON shape (updated, for full document fetch)

```json
{
  "id": 42,
  "parent_id": null,
  "title": "My Document",
  "body_html": "<p>Content</p>",
  "icon": "bx-file-blank",
  "position": 0,
  "version": 3,
  "is_favorite": false,
  "locked": false,
  "archived": false,
  "archived_at": "2026-05-01T12:00:00Z",
  "tags": [],
  "attachment_ids": [],
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-05-01T12:00:00Z",
  "assoc_year": null,
  "assoc_month": null,
  "assoc_day": null
}
```
