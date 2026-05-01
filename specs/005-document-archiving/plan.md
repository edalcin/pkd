# Implementation Plan: Document Archiving

**Branch**: `005-document-archiving` | **Date**: 2026-05-01 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/005-document-archiving/spec.md`

## Summary

Add document archiving to the PKD knowledge base: a new `archived_at` timestamp column on documents (mirroring `trashed_at`), two new API endpoints (archive/unarchive), a `view` parameter on the tree endpoint (active/archived/all), and frontend components for the view mode toggle, archive visual indicators, and read-only enforcement for archived documents.

## Technical Context

**Language/Version**: Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend)  
**Primary Dependencies**: chi v5 (router), modernc.org/sqlite v1.48.2, TipTap v2 (editor), Svelte 5 + Vite  
**Storage**: SQLite — single file, ISO-8601 strings for timestamps  
**Testing**: Go standard `testing` package  
**Target Platform**: Web (self-hosted, Linux server)  
**Project Type**: Web application (backend API + frontend SPA)  
**Performance Goals**: Tree view switches < 1 second (SC-002); archive/unarchive action < 5 seconds (SC-001)  
**Constraints**: CGO disabled (pure Go SQLite driver); no external services  
**Scale/Scope**: Personal knowledge base — single user, hundreds to low thousands of documents

## Constitution Check

Constitution file contains only template placeholders — no project-specific principles defined. No gates to evaluate.

## Project Structure

### Documentation (this feature)

```text
specs/005-document-archiving/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Research decisions
├── data-model.md        # DB + struct changes
├── quickstart.md        # End-to-end verification guide
├── contracts/
│   └── api.md           # API contracts
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code

```text
backend/
├── internal/
│   ├── model/
│   │   └── document.go          # Add ArchivedAt, Archived fields
│   ├── store/
│   │   ├── schema.sql           # Add archived_at column + index
│   │   └── documents.go         # Archive(), Unarchive(), ErrArchived, ListTree(view), Update()
│   └── server/
│       ├── handlers_documents.go  # handleArchiveDocument(), handleUnarchiveDocument()
│       ├── handlers_tree.go       # Pass view param to ListTree()
│       └── router.go              # Register new routes

frontend/
└── src/
    └── lib/
        ├── stores/
        │   └── documents.js         # archiveDoc(), unarchiveDoc(), viewMode store
        └── components/
            ├── Sidebar.svelte       # View mode toggle (Active/Archived/All)
            ├── TreeNode.svelte      # Archive action, archived visual indicator
            └── Editor.svelte        # Archived banner, read-only, unarchive button
```

**Structure Decision**: Web application (Option 2). Existing `backend/` + `frontend/` layout. No new directories needed.

## Implementation Phases

### Phase A: Database + Backend Store

**Files**: `internal/store/schema.sql`, `internal/model/document.go`, `internal/store/documents.go`

1. **schema.sql** — add `archived_at TEXT` column (after `locked`, before `trashed_at`) and index.
2. **document.go** — add `ArchivedAt *time.Time` and `Archived bool` to both `Document` and `DocumentTreeNode`.
3. **documents.go**:
   - Add `ErrArchived` sentinel.
   - Implement `Archive(id int64)`: fetch locked + archived_at, return ErrLocked / ErrArchived as needed, set `archived_at = now`.
   - Implement `Unarchive(id int64)`: clear `archived_at`, idempotent.
   - Update `Update()`: scan `archived_at` alongside `locked`; return `ErrArchived` if set.
   - Update `ListTree(view string, ...)`: switch query by view value. Active view uses recursive CTE when no search/tag filter; simple `archived_at IS NULL` filter when search/tags active. Archived view adds `AND archived_at IS NOT NULL`. All view has no archived_at filter.
   - Update `buildTree()` or `listByQuery()` scan to populate `ArchivedAt` and derive `Archived`.
   - Update `Restore()` (untrash): clear `archived_at` on restore to avoid a restored doc being silently archived.

### Phase B: Backend API Handlers + Routes

**Files**: `internal/server/handlers_documents.go`, `internal/server/handlers_tree.go`, `internal/server/router.go`

1. **handlers_documents.go**:
   - `handleArchiveDocument()`: parse ID, call `store.Archive()`, map ErrLocked→403, ErrArchived→409, ErrNotFound→404, write JSON.
   - `handleUnarchiveDocument()`: parse ID, call `store.Unarchive()`, map ErrNotFound→404, write JSON.
   - `handleUpdateDocument()`: map new `ErrArchived` → 423 with body `{"error":"document is archived"}`.
2. **handlers_tree.go**: read `view` query param (default `"active"`), pass to `store.ListTree()`.
3. **router.go**: register `POST /api/documents/{id}/archive` and `POST /api/documents/{id}/unarchive`.

### Phase C: Frontend Store

**File**: `frontend/src/lib/stores/documents.js`

1. Add `viewMode` writable store (default `'active'`).
2. Add `preSearchViewMode` variable (local module state).
3. Update `loadTree(options)` to pass `view` query param from `viewMode`.
4. Add `archiveDoc(id)`: POST `/api/documents/{id}/archive`, update tree node, trigger tree reload if current viewMode is `'active'` or `'archived'` (doc changes visibility).
5. Add `unarchiveDoc(id)`: POST `/api/documents/{id}/unarchive`, update tree node, trigger reload if needed.
6. Update search flow: on search start, save `preSearchViewMode`, set `viewMode` to `'all'`; on search clear, restore `preSearchViewMode`.

### Phase D: Frontend Components

**Files**: `Sidebar.svelte`, `TreeNode.svelte`, `Editor.svelte`

#### Sidebar.svelte — View mode toggle

- Add a segmented control or tab strip above the tree: **Active | Archived | All**.
- Binds to `viewMode` store.
- On viewMode change, call `loadTree()`.
- Hide when text filter is active (search takes over view mode).

#### TreeNode.svelte — Archive indicator + action

- Add archive action button to the row actions (alongside new-child, favorite, delete):
  - Shows "Archive" icon when `node.archived === false`; "Unarchive" icon when `node.archived === true`.
  - Disabled when `node.locked === true` (matches FR-017).
  - Calls `archiveDoc(node.id)` or `unarchiveDoc(node.id)`.
- Add visual indicator for archived nodes:
  - Apply a CSS class (e.g., `node-archived`) when `node.archived === true`.
  - `node-archived` styles: muted text color + archive icon (`bx-archive`) prefix.
  - Consistent in all views (Archived and All).

#### Editor.svelte — Read-only banner + unarchive action

- Add archived state to the existing `doc.locked` checks:
  - `editorInstance.setEditable(!doc?.locked && !doc?.archived && ...)` — disable editing when archived.
  - Disable title input, tags, icon picker when `doc.archived`.
- Add archived banner (above editor, similar pattern to any future notice area):
  - Shown when `doc.archived === true`.
  - Contains: archive icon + message "Este documento está arquivado." + **Desarquivar** button.
  - Clicking "Desarquivar" calls `unarchiveDoc(doc.id)`, updates local doc state.
- Apply visual treatment to toolbar when archived (similar to `toolbar-locked` class): opacity reduction, pointer-events none.

## Key Constraints & Notes

- **Recursive CTE**: SQLite supports recursive CTEs. The modernc.org/sqlite driver (pure Go) does support them — no CGO restriction applies to SQL features.
- **Archive vs Trash independence**: A document can be archived AND later trashed. `Restore()` (untrash) always clears `archived_at` to return a document to a clean active state.
- **Search scope**: The search endpoint (`GET /api/search`) is not modified — it already queries `body_text` and titles without an archived_at filter. The tree endpoint's search path (`q` param) must also omit the archived_at filter.
- **No orphan promotion in Active tree (non-search)**: The recursive CTE naturally prevents this. `buildTree()` needs no change for Active view without filters.

## Complexity Tracking

No constitution violations.
