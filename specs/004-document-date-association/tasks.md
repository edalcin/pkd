# Tasks: Document Date Association

**Input**: Design documents from `/specs/004-document-date-association/`  
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓, quickstart.md ✓

**Tests**: Not requested — no test tasks included.

**Organization**: Tasks grouped by user story (US1, US2, US3) for independent implementation and testing.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no shared dependencies)
- **[Story]**: User story this task belongs to (US1/US2/US3)
- Paths relative to `D:/git/pkd/`

---

## Phase 1: Setup

**Purpose**: Project structure is already established — no new directories or scaffolding needed.

- [x] T001 Confirm project builds cleanly: `go build ./...` from repo root and `npm run build` in `frontend/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema migration and Go data model changes that ALL user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 Add 3 idempotent `ALTER TABLE` entries to `colMigrations` slice in `internal/store/migrate.go` (`assoc_year INTEGER`, `assoc_month INTEGER`, `assoc_day INTEGER` — all nullable, follow existing pattern)
- [x] T003 [P] Add `AssocYear *int`, `AssocMonth *int`, `AssocDay *int` fields to `Document` struct in `internal/model/document.go` with JSON tags `assoc_year`, `assoc_month`, `assoc_day` (omitempty)
- [x] T004 [P] Update `scanDocRow` in `internal/store/documents.go`: add `assoc_year, assoc_month, assoc_day` to the SELECT query and add 3 `sql.NullInt64` scan targets; populate `doc.AssocYear/Month/Day` from valid values
- [x] T005 [P] Update `scanDocRows` in `internal/store/documents.go`: mirror the same SELECT and Scan additions as T004 for the list-query path

**Checkpoint**: `go build ./...` passes; `assoc_year/month/day` columns exist on DB open; `Document` struct serializes the new fields to JSON.

---

## Phase 3: User Story 1 — Associar data a um novo documento (Priority: P1) 🎯 MVP

**Goal**: User can set a partial or full date (year / month+year / day+month+year) on a document via the Associations panel. Invalid combinations are rejected. New documents pre-fill with today's date.

**Independent Test**: Create a new document → verify date pre-fills with today → change to year-only → save → reopen → confirm only year is shown. See `quickstart.md` tests 2–4.

### Implementation

- [x] T006 Add `UpdateAssocDate(id int64, year, month, day *int) (*model.Document, error)` method to `DocumentStore` in `internal/store/documents.go` (UPDATE assoc_year/month/day WHERE id AND trashed_at IS NULL; return updated doc via GetByID — see data-model.md)
- [x] T007 Add `handleUpdateAssocDate()` handler in `internal/server/handlers_documents.go`: parse `{year, month, day}` from JSON body; validate combinations (day requires month, month requires year, day in valid range for month/year) → 400 on violation; call `s.docs.UpdateAssocDate`; return updated doc — see contracts/associated-date.md
- [x] T008 Register route `r.Patch("/api/documents/{id}/associated-date", s.handleUpdateAssocDate())` in `internal/server/server.go` (alongside existing document routes)
- [x] T009 Add date section state variables to `<script>` block in `frontend/src/lib/components/Editor.svelte`: `let assocYear = null`, `let assocMonth = null`, `let assocDay = null`; populate from `doc.assoc_year/month/day` when `doc` changes
- [x] T010 [P] Add Year `<select>` dropdown to date section in `.assoc-area` in `frontend/src/lib/components/Editor.svelte`: options from 1900 to `new Date().getFullYear() + 10`; binds to `assocYear`
- [x] T011 [P] Add Month `<select>` dropdown to date section in `frontend/src/lib/components/Editor.svelte`: Portuguese names (Janeiro–Dezembro), values 1–12; binds to `assocMonth`; disabled when `assocYear` is null
- [x] T012 Add Day `<select>` dropdown to date section in `frontend/src/lib/components/Editor.svelte`: disabled when `assocMonth` is null; options 1 to N where N = days in `assocMonth`/`assocYear` (handle leap years: Feb 28 or 29); binds to `assocDay`
- [x] T013 Add Svelte reactive statement in `frontend/src/lib/components/Editor.svelte`: `$: if (!assocMonth) assocDay = null` (cascade: clearing month clears day automatically — FR-011a)
- [x] T014 Add "Limpar data" button in date section in `frontend/src/lib/components/Editor.svelte`: on click sets `assocYear = assocMonth = assocDay = null` and calls save function
- [x] T015 Pre-fill date state with today when `doc.assoc_year` is null (new document) in `frontend/src/lib/components/Editor.svelte`: in the `doc`-change reactive block, if `assocYear` remains null after loading, set year/month/day from `new Date()`
- [x] T016 Wire `saveAssocDate()` function in `frontend/src/lib/components/Editor.svelte`: calls `PATCH /api/documents/{doc.id}/associated-date` with `{year: assocYear, month: assocMonth, day: assocDay}`; invoke on each dropdown `on:change` event and on "Limpar data" click (immediate save, consistent with tags/links pattern)

**Checkpoint**: US1 fully functional — create doc → date pre-fills today → change to year-only → saves correctly → reopen shows year only.

---

## Phase 4: User Story 2 — Editar data associada de documento existente (Priority: P2)

**Goal**: Existing documents show their current associated date in the Associations panel (editable), with the immutable creation date displayed as read-only alongside it.

**Independent Test**: Open an existing document → verify current associated date is shown in dropdowns → change it → save → reopen → confirm new value persists; verify "Data de criação" is plain text only (no controls). See `quickstart.md` tests 1, 7, 8.

### Implementation

- [x] T017 Add contextual display label above the date dropdowns in `frontend/src/lib/components/Editor.svelte`: computed value shows "2024" (year-only), "Abril/2024" (month+year), or "25/04/2024" (full) based on which fields are set; shown in read/display context
- [x] T018 Add read-only "Data de criação" display in date section in `frontend/src/lib/components/Editor.svelte`: shows `doc.created_at` formatted as "DD/MM/YYYY HH:MM" as plain text; no edit controls — label + static value only (FR-007)

**Checkpoint**: US1 + US2 both work — existing doc shows correct associated date in dropdowns and immutable creation date as text; edits persist across page reloads.

---

## Phase 5: User Story 3 — Migração de documentos existentes (Priority: P3)

**Goal**: Documents created before this feature was deployed automatically receive their `created_at` date as the initial associated date. No manual action required from the user.

**Independent Test**: Stop app → delete `assoc_year/month/day` for a few rows directly in SQLite → restart app → verify those rows now have `assoc_year/month/day` set from their `created_at`. See `quickstart.md` test 1.

### Implementation

- [x] T019 Add data backfill migration block in `internal/store/migrate.go` (after the existing icon migration): `UPDATE documents SET assoc_year = CAST(strftime('%Y', created_at) AS INTEGER), assoc_month = CAST(strftime('%m', created_at) AS INTEGER), assoc_day = CAST(strftime('%d', created_at) AS INTEGER) WHERE assoc_year IS NULL` — idempotent; only touches rows added before this feature (FR-009)

**Checkpoint**: All pre-existing documents show their creation date as initial associated date without any user action.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: End-to-end validation and edge case verification across all stories.

- [ ] T020 [P] Run all 9 manual test scenarios in `specs/004-document-date-association/quickstart.md` — verify each passes; document any failures
- [ ] T021 [P] Verify edge case: Feb 29 appears in year dropdown for a leap year (e.g., 2024) and does not appear for a non-leap year (e.g., 2025) — test in `frontend/src/lib/components/Editor.svelte` day dropdown

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — **BLOCKS all user stories**
- **US1 (Phase 3)**: Depends on Phase 2 — backend (T006–T008) and frontend (T009–T016) can proceed in parallel once Phase 2 is done
- **US2 (Phase 4)**: Depends on Phase 3 (needs the frontend date section scaffold from T009/T010/T011)
- **US3 (Phase 5)**: Depends on Phase 2 (needs the 3 columns to exist) — can be done in parallel with Phase 3 or 4
- **Polish (Phase 6)**: Depends on all previous phases complete

### User Story Dependencies

- **US1 (P1)**: After Phase 2 — no dependency on US2 or US3
- **US2 (P2)**: After US1 frontend scaffold (T009–T012 must exist to add display label alongside dropdowns)
- **US3 (P3)**: After Phase 2 (ALTER TABLE) only — independent of US1 and US2

### Within US1

- T006 (store method) → T007 (handler uses store) → T008 (route registers handler)
- T009–T012 can be built in parallel (independent dropdown components)
- T013 depends on T009+T011 (cascade needs both month and day to exist)
- T014 depends on T009+T010+T011 (Limpar data clears all three)
- T015 depends on T009+T010+T011 (pre-fill sets all three)
- T016 (wire save) depends on T006+T007+T008+T009+T010+T011

---

## Parallel Opportunities

### Phase 2 — can run together once started

```
T003: Update scanDocRow (documents.go)
T004: Update scanDocRows (documents.go)  ← same file, coordinate line edits
T003 and T004 touch the same file — do sequentially or assign to one developer
T002: model/document.go — fully parallel with T003/T004
```

### Phase 3 — backend and frontend in parallel

```
Backend track:           Frontend track:
T006 store method        T009 Year dropdown
T007 handler         →   T010 Month dropdown
T008 route               T011 Day dropdown
                         T012 Day dropdown (depends on T010/T011)
                         T013 Cascade reactive
                         T014 Limpar button
                         T015 Pre-fill logic
                         T016 Wire PATCH call (depends on all above)
```

---

## Implementation Strategy

### MVP (User Story 1 only)

1. Phase 1: Confirm build ✓
2. Phase 2: Migrations + model + scan helpers (T002–T005)
3. Phase 3: Backend endpoint + frontend date UI (T006–T016)
4. **STOP and VALIDATE**: create doc, change date, save, reopen — verify
5. Deploy/demo with US1 working

### Incremental Delivery

1. Setup + Foundational → DB and model ready
2. US1 → can create/edit dates on new and existing documents
3. US2 → contextual display + read-only creation date (polish the UX)
4. US3 → existing docs auto-populated (run migration)
5. Polish → edge cases verified

---

## Notes

- `[P]` tasks touch different files or can be split between two developers
- US3 (T019) can be merged as soon as Phase 2 (T002) is complete — it only needs the columns
- The immediate-save pattern (on:change) is consistent with how tags and links work in the existing Associations panel
- `assocYear/Month/Day` state is the source of truth in the frontend; always sync from `doc.assoc_year/month/day` when the doc object is replaced
- Avoid bumping `version` in `UpdateAssocDate` — associated date is metadata, not document content
