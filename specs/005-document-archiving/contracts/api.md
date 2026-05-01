# API Contracts: Document Archiving

**Branch**: `005-document-archiving` | **Date**: 2026-05-01

## New Endpoints

### Archive a document

```
POST /api/documents/{id}/archive
```

**Path parameters**: `id` — document ID (integer)

**Request body**: empty

**Success response** `200 OK`:
```json
{
  "id": 42,
  "archived": true,
  "archived_at": "2026-05-01T12:34:56Z",
  ...
}
```

**Error responses**:
| Status | Condition |
|--------|-----------|
| 404 Not Found | Document does not exist or is trashed |
| 403 Forbidden | Document is locked (`ErrLocked`) |
| 409 Conflict | Document is already archived |

---

### Unarchive a document

```
POST /api/documents/{id}/unarchive
```

**Path parameters**: `id` — document ID (integer)

**Request body**: empty

**Success response** `200 OK`:
```json
{
  "id": 42,
  "archived": false,
  "archived_at": null,
  ...
}
```

**Error responses**:
| Status | Condition |
|--------|-----------|
| 404 Not Found | Document does not exist or is trashed |
| 409 Conflict | Document is already active (idempotent: may return 200 instead) |

---

## Modified Endpoints

### Get document tree

```
GET /api/tree
```

**New query parameter**: `view` (string, optional, default: `active`)

| Value | Documents returned |
|-------|--------------------|
| `active` | Non-archived, non-trashed. Children of archived parents are excluded. |
| `archived` | Archived, non-trashed only. |
| `all` | All non-trashed (active + archived). |

**Other query parameters** (unchanged):
- `tag` — filter by tag (repeatable, AND semantics)
- `favorite` — `1` to show only favorites
- `q` — full-text search query (ignores view filter for archived docs — searches all)

**Response shape**: unchanged (`DocumentTreeNode[]`), but nodes now include `archived` and `archived_at` fields.

**Behavior note**: When `q` is provided, the `view` parameter is ignored and results include both active and archived documents (FR-010/FR-011). The frontend is responsible for switching to `all` view when displaying search results.

---

### Update document content

```
PUT /api/documents/{id}
```

**New error response**:
| Status | Condition |
|--------|-----------|
| 423 Locked | Document is archived (read-only). Previously only returned for `locked=1`. |

Note: HTTP 423 (Locked) is used for both `ErrLocked` (user-initiated lock) and `ErrArchived` (archived = read-only), with distinct error body messages to allow the frontend to distinguish them.

**Error body format**:
```json
{ "error": "document is archived" }
```
vs existing:
```json
{ "error": "document is locked" }
```

---

## Frontend Store Functions (new)

```js
// archives a document; updates tree node with archived=true
export async function archiveDoc(id: number): Promise<Document>

// unarchives a document; updates tree node with archived=false
export async function unarchiveDoc(id: number): Promise<Document>
```

Both functions follow the same pattern as `toggleLock()`:
1. POST to backend
2. Update the in-memory `tree` store with the returned document state
3. If the current view mode is `active`, trigger a tree reload (archived doc disappears from view)

---

## Frontend View Mode Store

```js
// viewMode: 'active' | 'archived' | 'all'
// Default: 'active', resets on page load
export const viewMode = writable('active')

// preSearchViewMode: stored before search activates, restored on clear
let preSearchViewMode = 'active'
```

`loadTree()` passes `viewMode` as the `view` query parameter to `GET /api/tree`.
