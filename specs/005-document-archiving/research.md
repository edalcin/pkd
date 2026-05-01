# Research: Document Archiving

**Branch**: `005-document-archiving` | **Date**: 2026-05-01

## Summary

No external unknowns. All decisions resolved from existing codebase patterns and spec clarifications.

---

## Decision 1: Archive State Storage

**Decision**: Add `archived_at TEXT` nullable column to `documents` table (ISO-8601 timestamp), mirroring the existing `trashed_at` soft-delete pattern.

**Rationale**: Identical to the trashed_at approach already in the codebase. Null = active, non-null = archived. Stores when archiving happened for free. No new tables or joins required.

**Alternatives considered**:
- Separate `status` enum column ("active" | "archived" | "trashed"): Rejected — trashed_at and archived_at serve different workflows; a doc should be archivable independently of trash state. An enum would conflate two separate concepts.
- Boolean `is_archived` column: Rejected — loses timestamp; trashed_at pattern sets precedent and stores useful metadata.

---

## Decision 2: Active Tree — Hiding Children of Archived Parents

**Decision**: Use a recursive CTE for the Active tree query (no search/tag filter) to fetch only docs whose entire ancestor chain is non-archived. For search/tag-filtered Active queries, apply simple `archived_at IS NULL` filter and allow orphan promotion (current behavior preserved).

**Rationale**: A recursive CTE natively traverses the parent chain and stops at archived ancestors. This is the only correct way to implement "children hidden when parent archived" without application-layer post-processing. SQLite (modernc.org/sqlite) supports recursive CTEs via FTS5/WAL mode, which this project already uses.

**Alternatives considered**:
- Application-layer post-processing (fetch all, then walk tree and prune): Rejected — O(n) in-memory traversal for potentially large trees; recursive CTE is O(depth) and done in one query.
- Denormalized `has_archived_ancestor` column: Rejected — requires updating all descendants on every archive/unarchive operation; fragile and complex.

---

## Decision 3: Read-Only Enforcement for Archived Documents

**Decision**: Add `ErrArchived` sentinel error in the store layer. `Update()` checks `archived_at IS NOT NULL` and returns `ErrArchived` (HTTP 423 Locked, or reuse 403 Forbidden). Frontend disables editor and toolbar when `doc.archivedAt !== null`, matching the existing `doc.locked` pattern exactly.

**Rationale**: The codebase already has `ErrLocked` for the same purpose. Adding `ErrArchived` keeps the error model consistent. Frontend already has a pattern for disabling the editor via `editorInstance.setEditable(false)`.

**Alternatives considered**:
- Reuse `ErrLocked` for archived docs: Rejected — conflates two different states with different user-facing messages and different resolution paths (unlock vs unarchive).

---

## Decision 4: Archive Action Blocked When Locked

**Decision**: `Archive()` store method checks `locked = 1` and returns `ErrLocked` (HTTP 403). Frontend disables the archive action in TreeNode and Editor when `node.locked || doc.locked`.

**Rationale**: Directly matches FR-017 and the established `SoftDelete()` pattern, which also blocks on `ErrLocked`.

---

## Decision 5: New API Endpoints

**Decision**: Two new endpoints: `POST /api/documents/{id}/archive` and `POST /api/documents/{id}/unarchive`. Tree endpoint updated: `GET /api/tree?view=active|archived|all` (default: `active`). Search continues to use `GET /api/search?q=...` unchanged; backend query updated to remove `archived_at IS NULL` filter so archived docs are included in search results.

**Rationale**: Follows existing `POST /api/documents/{id}/lock` pattern for toggle-style actions. Separate archive/unarchive endpoints (vs a toggle) are more explicit and easier to reason about for frontend state management. Tree `view` parameter mirrors how `q`, `tag`, and `favorite` params already work.

---

## Decision 6: View Mode State (Frontend)

**Decision**: View mode (`active` | `archived` | `all`) stored in a Svelte writable store (`viewMode`). Resets to `active` on page load (no persistence). When search is active, tree auto-switches to `all`; when search is cleared, tree reverts to pre-search `viewMode`.

**Rationale**: Matches spec (FR-013: default active on load, SC-005: revert on clear). No localStorage persistence needed per clarification. Store approach integrates cleanly with existing `tree` store and reactive `$effect` patterns in Sidebar.svelte.

---

## Decision 7: Unarchive from Document Content View

**Decision**: Add an "Unarchive" button inside the archived document banner in `Editor.svelte`. This matches FR-016 (unarchive from content view) and mirrors how the lock button is placed in the editor header.

**Rationale**: User should not need to close the document and navigate to the tree to unarchive. The banner is already the prominent indicator of archived state, making it the natural location for the unarchive action.
