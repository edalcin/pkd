# Tasks: Document Archiving

**Input**: Design documents from `/specs/005-document-archiving/`  
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/api.md ✓, quickstart.md ✓

**Tests**: No test tasks — not requested in specification.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies between them)
- **[Story]**: Which user story this task belongs to (US1–US4)

---

## Phase 1: Setup

**Purpose**: No new project scaffolding needed — existing mature codebase. Phase 1 is the DB migration that unblocks all other work.

- [x] T001 [P] Add `archived_at TEXT` column and `idx_documents_archived_at` index to `internal/store/schema.sql` (after `locked` column, before `trashed_at`)
- [x] T002 [P] Add `ArchivedAt *time.Time` and `Archived bool` fields to `Document` struct and `DocumentTreeNode` struct in `internal/model/document.go`; populate `Archived` as `ArchivedAt != nil`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Store-layer error infrastructure that every user story depends on.

**⚠️ CRITICAL**: Phase 3+ cannot begin until T001, T002, T003, and T004 are complete.

- [x] T003 Add `ErrArchived = errors.New("document is archived")` sentinel and update `Update()` in `internal/store/documents.go` to scan `archived_at` and return `ErrArchived` if set (check archived before checking locked)
- [x] T004 Update `Restore()` (untrash) in `internal/store/documents.go` to also clear `archived_at` (set to NULL) so restored documents always return to active state

**Checkpoint**: Store error infrastructure ready — user story work can now begin.

---

## Phase 3: User Story 1 — Archive a Document (Priority: P1) 🎯 MVP

**Goal**: User can archive any active, unlocked document; it disappears from the active tree; opening it shows a read-only banner.

**Independent Test**: Archive a document → confirm it vanishes from active tree → switch to archived view → confirm it appears with visual indicator → open it → confirm banner + editor disabled.

### Implementation

- [x] T005 [US1] Implement `Archive(id int64)` method in `internal/store/documents.go`: fetch `locked` and `archived_at`, return `ErrLocked` if locked, `ErrArchived` if already archived, else set `archived_at = now` and return updated document
- [x] T006 [US1] Add `handleArchiveDocument()` handler in `internal/server/handlers_documents.go`: parse `{id}`, call `store.Archive()`, map `ErrNotFound→404`, `ErrLocked→403`, `ErrArchived→409`, write JSON response
- [x] T007 [US1] Update `handleUpdateDocument()` in `internal/server/handlers_documents.go` to map `ErrArchived` to HTTP 423 with body `{"error":"document is archived"}` (distinct from existing `ErrLocked→403`)
- [x] T008 [US1] Register `POST /api/documents/{id}/archive` route in `internal/server/router.go` pointing to `handleArchiveDocument()`
- [x] T009 [US1] Add `archiveDoc(id)` async function to `frontend/src/lib/stores/documents.js`: POST to `/api/documents/{id}/archive`, update tree node in `tree` store with returned `archived`/`archived_at` values, trigger `loadTree()` reload so the doc disappears from current view
- [x] T010 [US1] Add archive action button to `frontend/src/lib/components/TreeNode.svelte`: show archive icon button in row actions (disabled when `node.locked`); on click, call `archiveDoc(node.id)`; apply `node-archived` CSS class when `node.archived` (muted color + `bx-archive` icon prefix on label)
- [x] T011 [US1] Add archived state to `frontend/src/lib/components/Editor.svelte`: include `doc?.archived` in the `setEditable()` condition (editor disabled when archived); add archived banner element shown when `doc.archived` with message "Este documento está arquivado." and disabled toolbar class

**Checkpoint**: User Story 1 fully functional — archive/visual indicator/read-only all work independently of the view toggle.

---

## Phase 4: User Story 2 — Switch Document Tree Views (Priority: P2)

**Goal**: User can toggle the tree between Active, Archived, and All views; default on load is Active.

**Independent Test**: With one active and one archived document, cycle through all three view modes and confirm each shows only the expected documents; reload page and confirm Active is the default.

### Implementation

- [x] T012 [US2] Update `ListTree(view string, tagFilter []string, favoriteOnly bool, q string)` in `internal/store/documents.go`:
  - `view="active"` with no search/tag filter: use recursive CTE (seed: root docs with `archived_at IS NULL AND trashed_at IS NULL`; recurse: children of active parents with same conditions)
  - `view="active"` with search or tag filter: add `AND archived_at IS NULL` to existing flat query (preserve current orphan-promotion behavior)
  - `view="archived"`: `WHERE archived_at IS NOT NULL AND trashed_at IS NULL`
  - `view="all"`: `WHERE trashed_at IS NULL` (no `archived_at` filter)
  - All scan paths must populate `ArchivedAt` and derive `Archived` on each returned node
- [x] T013 [P] [US2] Update `handleGetTree()` in `internal/server/handlers_tree.go` to read `view` query param (default `"active"`) and pass it as first argument to `store.ListTree()`
- [x] T014 [P] [US2] Add `viewMode` writable store (default `'active'`) to `frontend/src/lib/stores/documents.js`; update `loadTree()` to include `view=${viewMode}` in the `GET /api/tree` query string
- [x] T015 [US2] Add Active / Archived / All segmented toggle above the document tree in `frontend/src/lib/components/Sidebar.svelte`: binds to `viewMode` store, calls `loadTree()` on change, hide toggle element while text filter/search is active

**Checkpoint**: User Stories 1 and 2 both independently functional — tree view switching works.

---

## Phase 5: User Story 3 — Unarchive a Document (Priority: P3)

**Goal**: User can restore an archived document to active status from either the tree or the document content view.

**Independent Test**: Archive a document → switch to Archived view → unarchive it → confirm it disappears from Archived tree → switch to Active view → confirm it reappears editable.

### Implementation

- [x] T016 [US3] Implement `Unarchive(id int64)` method in `internal/store/documents.go`: fetch document, return `ErrNotFound` if absent or trashed, clear `archived_at` (set NULL), return updated document (idempotent — no error if already active)
- [x] T017 [P] [US3] Add `handleUnarchiveDocument()` handler in `internal/server/handlers_documents.go`: parse `{id}`, call `store.Unarchive()`, map `ErrNotFound→404`, write JSON response
- [x] T018 [P] [US3] Register `POST /api/documents/{id}/unarchive` route in `internal/server/router.go` pointing to `handleUnarchiveDocument()`
- [x] T019 [P] [US3] Add `unarchiveDoc(id)` async function to `frontend/src/lib/stores/documents.js`: POST to `/api/documents/{id}/unarchive`, update tree node with returned state, trigger `loadTree()` reload
- [x] T020 [US3] Update `frontend/src/lib/components/TreeNode.svelte`: when `node.archived`, swap the archive button for an "Unarchive" button (calls `unarchiveDoc(node.id)`); archive button shown only for non-archived nodes
- [x] T021 [US3] Add "Desarquivar" button to the archived document banner in `frontend/src/lib/components/Editor.svelte`: on click, call `unarchiveDoc(doc.id)`, which updates the store; the banner hides reactively when `doc.archived` becomes false

**Checkpoint**: User Stories 1, 2, and 3 all functional — full archive/unarchive roundtrip works from tree and from content view.

---

## Phase 6: User Story 4 — Search Across All Documents (Priority: P4)

**Goal**: The "Filtrar..." bar returns results from both active and archived documents; tree auto-switches to All view for results; reverts to previous view on clear.

**Independent Test**: Archive a document → type its title in Filter bar → confirm it appears in results with archived indicator → confirm tree is in All view → clear search → confirm tree returns to previous view.

### Implementation

- [x] T022 [US4] Verify and update `internal/store/search.go` (or equivalent search query function): ensure the query that backs `/api/search` does NOT filter by `archived_at IS NULL` so dropdown results include archived documents
- [x] T023 [P] [US4] Update search flow in `frontend/src/lib/stores/documents.js`: when search/filter becomes active, save current `viewMode` to `preSearchViewMode` and set `viewMode` to `'all'`; when search is cleared, restore `viewMode` from `preSearchViewMode`; ensure `loadTree()` is called after each viewMode change
- [x] T024 [US4] Update `frontend/src/lib/components/Sidebar.svelte` (and/or `Search.svelte`): integrate with the search-triggered viewMode switch from T023; ensure the view toggle is hidden/disabled while search is active (not just cosmetically — prevent manual mode switch during search)

**Checkpoint**: All 4 user stories complete — full feature functional end-to-end.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [x] T025 [P] Add empty-state message to `frontend/src/lib/components/Sidebar.svelte` for when the current view has no documents (e.g., "Nenhum documento ativo." for Active view, "Nenhum documento arquivado." for Archived view)
- [x] T026 [P] Verify archived documents appear correctly in the graph/link visualization (`frontend/src/lib/components/Graph.svelte` or equivalent): no changes needed per plan, but confirm graph still loads and renders archived document nodes without errors
- [ ] T027 Run the end-to-end smoke tests from `specs/005-document-archiving/quickstart.md` and fix any issues found

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately; T001 and T002 are parallel
- **Phase 2 (Foundational)**: Depends on Phase 1 — T003 requires T002 (ArchivedAt field); T004 independent of T003
- **Phase 3 (US1)**: Depends on Phase 2 completion — BLOCKS nothing else once complete
- **Phase 4 (US2)**: Depends on Phase 2; T012 is the heaviest backend task in this feature
- **Phase 5 (US3)**: Depends on Phase 2 and Phase 3 (reuses archive visual components in Editor/TreeNode)
- **Phase 6 (US4)**: Depends on Phase 4 (viewMode store must exist); T022 is independent of viewMode
- **Phase 7 (Polish)**: Depends on all stories complete

### User Story Dependencies

- **US1 (P1)**: After Phase 2 — no dependencies on other stories
- **US2 (P2)**: After Phase 2 — independent of US1 (can start in parallel with US1 once foundation is done)
- **US3 (P3)**: After US1 — reuses archived banner in Editor (T011) for unarchive button (T021)
- **US4 (P4)**: After US2 — viewMode store (T014) must exist before search viewMode switching (T023)

### Within Each User Story

- Store method → HTTP handler → route registration → frontend store → frontend component
- Backend and frontend can proceed in parallel for different files once the contract (api.md) is agreed

---

## Parallel Opportunities

### Phase 1 (run together)
```
T001 — schema.sql (add column + index)
T002 — document.go (add struct fields)
```

### Phase 3 US1 (backend/frontend split)
```
Backend:  T005 → T006 → T007 → T008
Frontend: T009 → T010 → T011  (start after T005 interface is known)
```

### Phase 4 US2 (backend/frontend split after T012)
```
Backend:  T012 → T013
Frontend: T014 (parallel with T013) → T015
```

### Phase 5 US3 (after T016)
```
T017, T018, T019 — all parallel (different files, all depend on T016)
Then: T020, T021 in sequence (same component files)
```

### Phase 7 Polish (all parallel)
```
T025, T026 — parallel
T027 — after T025, T026
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: T001, T002 (parallel)
2. Complete Phase 2: T003, T004
3. Complete Phase 3: T005 → T006 → T007 → T008 → T009 → T010 → T011
4. **STOP and VALIDATE**: Archive a document, confirm visual indicator, confirm read-only
5. This delivers: archive action, visual differentiation, read-only enforcement

### Incremental Delivery

1. Phase 1+2 → Foundation ready
2. Phase 3 (US1) → **MVP**: Archive works, visually indicated, read-only enforced
3. Phase 4 (US2) → View toggle: Active / Archived / All
4. Phase 5 (US3) → Unarchive from tree and from content view
5. Phase 6 (US4) → Search includes archived docs, auto-switches to All view
6. Phase 7 → Polish and smoke tests

---

## Notes

- [P] = different files, no incomplete-task dependencies
- [USN] maps task to user story for traceability
- T012 (ListTree recursive CTE) is the most complex single task — allocate extra time
- T003 touches the hot path (`Update()` is called on every save) — test carefully
- Commit after each checkpoint to enable easy rollback per story
