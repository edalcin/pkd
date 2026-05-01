# Quickstart: Document Archiving

**Branch**: `005-document-archiving` | **Date**: 2026-05-01

## How to verify end-to-end after implementation

### 1. Apply the DB migration

```bash
# The app initializes schema on start — ensure the new column is in schema.sql
# For an existing database, run:
sqlite3 data/pkd.db "ALTER TABLE documents ADD COLUMN archived_at TEXT;"
sqlite3 data/pkd.db "CREATE INDEX IF NOT EXISTS idx_documents_archived_at ON documents(archived_at);"
```

### 2. Build and run

```bash
# Backend
cd backend && go build ./... && go run ./cmd/server

# Frontend
cd frontend && npm run dev
```

### 3. Smoke test: Archive workflow

1. Open the app in a browser.
2. In the document tree (default "Active" view), right-click or use the action menu on any document.
3. Select "Archive" — document disappears from the Active tree immediately.
4. Click the tree view toggle → switch to "Archived" view.
5. Confirm the document appears there with a visual indicator (muted color / archive icon).
6. Click the document — confirm a banner shows "This document is archived" and the editor is read-only (toolbar disabled).
7. Click the "Unarchive" button in the banner → document moves back to Active tree.
8. Switch to "Active" view — document is visible and editable again.

### 4. Smoke test: Parent-child hierarchy

1. Create a parent document with two child documents.
2. Archive the parent only.
3. Switch to "Active" view — parent AND both children should be hidden.
4. Switch to "All" view — parent (archived, visually muted) and children (active, normal) all visible.
5. Unarchive the parent — switch back to "Active" view — all three reappear.

### 5. Smoke test: Search across all documents

1. Archive a document.
2. Use the "Filtrar..." search bar and type part of the archived document's title.
3. Confirm the archived document appears in results (with archive indicator).
4. Confirm the tree has automatically switched to "All" view.
5. Clear the search — confirm the tree reverts to the previous view mode.

### 6. Smoke test: Lock interaction

1. Lock a document (using the lock button in the editor).
2. Try to archive it from the tree — confirm an error message appears ("Destranque o documento antes de arquivar").
3. Unlock the document, then archive successfully.

## Key files changed

| File | What changed |
|------|-------------|
| `internal/store/schema.sql` | Add `archived_at TEXT` column and index |
| `internal/model/document.go` | Add `ArchivedAt *time.Time`, `Archived bool` fields |
| `internal/store/documents.go` | New `Archive()`, `Unarchive()`, `ErrArchived`; updated `Update()`, `ListTree()` |
| `internal/server/handlers_documents.go` | New `handleArchiveDocument()`, `handleUnarchiveDocument()` handlers |
| `internal/server/handlers_tree.go` | Pass `view` param to `ListTree()` |
| `internal/server/router.go` | Register new archive/unarchive routes |
| `frontend/src/lib/stores/documents.js` | New `archiveDoc()`, `unarchiveDoc()`, `viewMode` store |
| `frontend/src/lib/components/Sidebar.svelte` | View mode toggle (Active/Archived/All) |
| `frontend/src/lib/components/TreeNode.svelte` | Archive action, visual indicator for archived nodes |
| `frontend/src/lib/components/Editor.svelte` | Archived banner, read-only mode for archived docs |
