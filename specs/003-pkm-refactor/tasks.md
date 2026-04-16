# Tasks: PKM Refactor (003-pkm-refactor)

**Input**: Design documents from `/specs/003-pkm-refactor/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml, quickstart.md
**Date**: 2026-04-16

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Svelte Frontend Scaffold)

**Purpose**: Initialize the Svelte frontend project alongside the existing Go backend

- [X] T001 Initialize Svelte 5 + Vite project in `frontend/` with `package.json`, `vite.config.js`, `svelte.config.js`
- [X] T002 Install frontend dependencies: `svelte-tiptap`, `@tiptap/core`, `@tiptap/starter-kit`, `@tiptap/extension-image`, `@tiptap/suggestion`, `d3-force`, `d3-selection`, `d3-zoom`, `d3-drag`
- [X] T003 [P] Configure Vite build output to `internal/server/web/dist/` in `frontend/vite.config.js`
- [X] T004 [P] Create `frontend/src/main.js` entry point and `frontend/src/App.svelte` shell layout (sidebar + content area)
- [X] T005 [P] Create `frontend/src/styles/app.css` with CSS custom properties for light/dark theme tokens
- [X] T006 [P] Create `frontend/public/` directory with PWA icons and `manifest.webmanifest` placeholder

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

- [X] T007 Add `document_links` table DDL to `internal/store/schema.sql` per data-model.md (CREATE TABLE IF NOT EXISTS, indexes on source_id and target_id)
- [X] T008 Add migration logic for `document_links` table in `internal/store/migrate.go` (idempotent, safe for existing databases)
- [X] T009 [P] Create `internal/model/link.go` with `Link` struct (`ID`, `SourceID`, `TargetID`, `CreatedAt`) and `LinkEntry` response struct (`SourceTitle`, `TargetTitle`, `TargetTrashed`)
- [X] T010 [P] Create `frontend/src/lib/api.js` — fetch wrapper with CSRF token handling (read `pkd_csrf` cookie, set `X-CSRF-Token` header), JSON helpers, error handling
- [X] T011 [P] Create `frontend/src/lib/stores/auth.js` — Svelte store for auth state, login/logout API calls
- [X] T012 Create `frontend/src/lib/components/LoginPage.svelte` — login form calling `POST /api/login` via api.js
- [X] T013 Implement hash-based router in `frontend/src/App.svelte` — routes: `#/`, `#/doc/{id}`, `#/graph`, `#/calendar`, `#/admin`, `#/search` per research.md Decision 8
- [X] T014 Update `internal/server/assets.go` to embed `web/dist/` instead of current vanilla JS files (or alongside, with dist/ taking precedence)
- [X] T015 Update `internal/server/server.go` to serve `index.html` from embedded Svelte build for the root route and frontend routes

**Checkpoint**: Frontend scaffold connects to backend, login works, hash routing navigates between placeholder views

---

## Phase 3: User Story 1 — Criar e editar documentos ricos (Priority: P1) MVP

**Goal**: Users can create, edit, and organize rich documents in a hierarchical tree with TipTap editor, auto-save, image support, and drag-drop reordering.

**Independent Test**: Create a root document, add 3 levels of nested children, format text with headings/bold/lists/code, paste and resize an image, save, reload — everything persists.

### Implementation for User Story 1

- [X] T016 [P] [US1] Create `frontend/src/lib/stores/documents.js` — Svelte store for document tree, selected document, CRUD operations via api.js (GET /api/tree, POST /api/documents, PUT /api/documents/{id}, DELETE /api/documents/{id})
- [X] T017 [P] [US1] Create `frontend/src/lib/stores/tags.js` — Svelte store for tag list, document tag operations (GET /api/tags, PUT /api/documents/{id}/tags)
- [X] T018 [US1] Create `frontend/src/lib/components/Sidebar.svelte` — document tree with expandable nodes, icons, drag-drop reordering (POST /api/documents/{id}/move), "New Document" button, tag filter chips
- [X] T019 [US1] Create `frontend/src/lib/components/Editor.svelte` — TipTap v2 wrapper using `svelte-tiptap` with StarterKit, Image extension (resize handles), auto-save (debounce 2s on content change → PUT /api/documents/{id}), version conflict detection (409 → overwrite/reload dialog)
- [X] T020 [US1] Implement document metadata header in `Editor.svelte` — editable title, icon picker (emoji or icon set), tag input with autocomplete
- [X] T021 [US1] Implement image upload in `Editor.svelte` — paste/drop image → POST /api/documents/{id}/attachments → insert inline with resize handles via TipTap Image extension
- [X] T022 [US1] Implement file attachment UI in `Editor.svelte` — attachment list below editor, upload button → POST /api/documents/{id}/attachments, download link, delete button
- [X] T023 [US1] Implement trash functionality — soft-delete from sidebar (DELETE /api/documents/{id}), restore from trash view (POST /api/documents/{id}/restore)
- [X] T024 [US1] Wire document selection: clicking a tree node navigates to `#/doc/{id}`, loads document in Editor

**Checkpoint**: Full document CRUD, rich editing, image upload/resize, hierarchy navigation, auto-save with version conflict handling

---

## Phase 4: User Story 2 — Conectar documentos com links bidirecionais (Priority: P1)

**Goal**: Users can link documents with `[[` syntax in the editor. Backlinks appear automatically. Links to trashed docs show as broken.

**Independent Test**: Create docs A, B, C. Link A→B and A→C via `[[` autocomplete. Verify B and C show "Referenced by A". Delete link A→B. Verify B no longer shows backlink.

### Implementation for User Story 2

- [X] T025 [P] [US2] Create `internal/store/links.go` — CRUD for `document_links` table: `CreateLink(sourceID, targetID)`, `DeleteLink(id)`, `GetLinksForDocument(id)` (returns outgoing + incoming with titles and trashed status), `GetAllLinksForGraph()`, `SyncLinksFromHTML(docID, html)` (parse `data-doc-link` attributes, diff, insert/delete within transaction)
- [X] T026 [P] [US2] Create `internal/server/handlers_links.go` — REST endpoints: `GET /api/documents/{id}/links`, `POST /api/documents/{id}/links`, `DELETE /api/documents/{id}/links/{linkId}` per openapi.yaml
- [X] T027 [US2] Extend `PUT /api/documents/{id}` handler in `internal/server/handlers_documents.go` — after saving body, call `SyncLinksFromHTML` to update `document_links` table within the same transaction
- [X] T028 [US2] Build TipTap `docLink` extension in `frontend/src/lib/editor/doclink-extension.js` — custom inline Node type with `data-doc-link` attribute storing target document ID, renders as styled `<span>` with document title, click navigates to `#/doc/{id}`
- [X] T029 [US2] Build TipTap `[[` autocomplete in `frontend/src/lib/editor/link-suggestion.js` — uses `@tiptap/suggestion`, triggers on `[[`, queries `GET /api/search?q={query}&limit=10` (debounced 150ms), inserts `docLink` node on selection
- [X] T030 [US2] Integrate docLink extension and link-suggestion into `Editor.svelte` — add to TipTap extensions array
- [X] T031 [US2] Create backlinks panel in `Editor.svelte` — section "Referenced by" below editor, fetches `GET /api/documents/{id}/links`, shows incoming links as clickable items, marks links to trashed docs as "broken" (strikethrough + icon)

**Checkpoint**: `[[` autocomplete inserts links, backlinks panel shows reverse references, link sync on save, broken link visual for trashed targets

---

## Phase 5: User Story 3 — Buscar e filtrar o conhecimento (Priority: P1)

**Goal**: Universal search finds documents by substring of title, body, or tag. Tree can be filtered by one or more tags (AND semantics).

**Independent Test**: Create 10 docs with distinct text and tags. Search by body substring — correct results with snippets. Filter tree by tag — only matching docs shown.

### Implementation for User Story 3

- [X] T032 [P] [US3] Create `frontend/src/lib/stores/search.js` — Svelte store for search query, results (from GET /api/search?q=...), debounced at 150ms
- [X] T033 [US3] Create `frontend/src/lib/components/Search.svelte` — search input in header/sidebar, results dropdown with title + snippet (highlighted match), click navigates to `#/doc/{id}`
- [X] T034 [US3] Implement tag filter in `Sidebar.svelte` — clickable tag chips from tags store, selecting tags filters the tree via `GET /api/tree?tag=tagname` (AND semantics for multiple tags)
- [X] T035 [US3] Create `#/search` route view in `App.svelte` — full search results page for when user presses Enter (not just dropdown), showing all matching documents with snippets

**Checkpoint**: Real-time search with snippets, tag-based tree filtering, full search results page

---

## Phase 6: User Story 4 — Visualizar conexoes em grafo (Priority: P2)

**Goal**: Interactive D3.js force-directed graph showing documents as nodes and links as edges. Zoom, pan, click-to-navigate. Tag-based coloring. Only connected docs shown by default.

**Independent Test**: Create 5 docs with links. Open Graph View. All connected nodes and edges render. Click node opens doc. Zoom/pan works. Toggle shows isolated docs.

### Implementation for User Story 4

- [X] T036 [P] [US4] Create `internal/server/handlers_graph.go` — `GET /api/graph` endpoint returning `{nodes: [{id, title, icon, tags}], edges: [{source, target}]}` per openapi.yaml. Default: only docs with ≥1 link. Support `?tag=...` filter and `?all=true` for all docs.
- [X] T037 [US4] Create `frontend/src/lib/components/GraphView.svelte` — D3.js force-directed graph per research.md Decision 3: `d3-force` simulation updates node/link arrays, Svelte `{#each}` renders SVG `<circle>` and `<line>`, `d3-zoom` and `d3-drag` via Svelte `use:` actions
- [X] T038 [US4] Implement tag-based node coloring in `GraphView.svelte` — assign color from a palette based on primary tag, add legend
- [X] T039 [US4] Implement graph controls in `GraphView.svelte` — tag filter dropdown, "show all documents" toggle, zoom-to-fit button
- [X] T040 [US4] Wire node click in `GraphView.svelte` — clicking a node navigates to `#/doc/{id}`

**Checkpoint**: Graph renders with force layout, nodes colored by tag, interactive zoom/pan/drag, click navigates, tag filter works

---

## Phase 7: User Story 5 — Capturar conteudo externo (Priority: P2)

**Goal**: Capture external content via API and PWA share target. URLs get Open Graph metadata extraction. Documents created with `#captura` tag.

**Independent Test**: POST to `/api/capture` with a URL — document created with OG title and `#captura` tag. Share from mobile browser to PKD PWA — same result.

### Implementation for User Story 5

- [X] T041 [P] [US5] Create `internal/server/handlers_capture.go` — `POST /api/capture` endpoint per openapi.yaml: accepts `{title, content, url, tags}`, creates document with default tag `#captura`, supports both `application/json` and `application/x-www-form-urlencoded` (for PWA share_target)
- [X] T042 [P] [US5] Implement Open Graph extraction in `internal/server/handlers_capture.go` — when `url` is provided, HTTP GET with 5s timeout, parse `<meta property="og:*">` using `golang.org/x/net/html`, extract title/description/image (best-effort, fail silently)
- [X] T043 [US5] Update `manifest.webmanifest` with `share_target` configuration per research.md Decision 5 — action: `/api/capture`, method: POST, enctype: `application/x-www-form-urlencoded`, params: title, text, url
- [X] T044 [US5] Update service worker `sw.js` to intercept share_target POST requests, attach auth cookie, and forward to `/api/capture`

**Checkpoint**: API capture creates documents with OG metadata, mobile share target sends content to PKD

---

## Phase 8: User Story 6 — Compartilhar documentos via link publico (Priority: P2)

**Goal**: Generate unique public share links per document. Public view is read-only with restricted CSP. Revoked links return 404.

**Independent Test**: Generate share link. Open in private window — content renders read-only. Revoke link. Same URL returns 404.

### Implementation for User Story 6

- [X] T045 [US6] Create `frontend/src/lib/components/ShareDialog.svelte` — dialog triggered from Editor toolbar, calls `POST /api/documents/{id}/shares`, shows generated URL with copy button, lists existing shares with revoke button (`DELETE /api/documents/{id}/shares/{shareId}`)
- [X] T046 [US6] Create `frontend/src/lib/components/ShareView.svelte` — standalone read-only document renderer for `/public/{token}` route, minimal CSS, no sidebar/navigation, restricted CSP applied by backend
- [X] T047 [US6] Wire ShareDialog into Editor.svelte — share button in document header/toolbar opens ShareDialog

**Checkpoint**: Share links generated, public view works read-only, revoke returns 404

---

## Phase 9: User Story 7 — Navegar por calendario (Priority: P3)

**Goal**: Calendar view shows documents organized by creation date. Click a day to see documents created that day.

**Independent Test**: Create documents on different dates. Open calendar. Each day shows correct document count. Click a day — documents listed.

### Implementation for User Story 7

- [X] T048 [US7] Create `frontend/src/lib/components/Calendar.svelte` — month grid calendar fetching `GET /api/calendar/{year}/{month}`, days with documents show count badge, clicking a day expands to show document list with links to `#/doc/{id}`, prev/next month navigation

**Checkpoint**: Calendar renders months, days with documents highlighted, click shows document list

---

## Phase 10: User Story 8 — Administracao e manutencao (Priority: P3)

**Goal**: Admin panel with backup/restore, orphan cleanup, tag rename/merge, and trash management.

**Independent Test**: Backup, change data, restore — state reverted. Rename tag across N documents — all updated. Empty trash — documents permanently gone.

### Implementation for User Story 8

- [X] T049 [P] [US8] Create `frontend/src/lib/components/Admin.svelte` — admin panel layout with tabs/sections for: Backup/Restore, Trash, Tags, Cleanup
- [X] T050 [P] [US8] Implement backup/restore section in `Admin.svelte` — backup button triggers `POST /api/admin/backup` (file download), restore uploads file via `POST /api/admin/restore` with `confirm=REPLACE`
- [X] T051 [P] [US8] Implement trash management in `Admin.svelte` — list trashed docs via `GET /api/admin/trash`, restore button per doc (`POST /api/documents/{id}/restore`), permanent delete per doc (`DELETE /api/admin/trash/{id}`), empty all button (`POST /api/admin/trash/empty`)
- [X] T052 [P] [US8] Implement tag management in `Admin.svelte` — list tags via `GET /api/tags`, rename/merge via `PUT /api/admin/tags/rename` with old/new inputs, show document count per tag
- [X] T053 [US8] Implement cleanup section in `Admin.svelte` — button to run `POST /api/admin/cleanup`, display result (orphans removed count)

**Checkpoint**: All admin operations work: backup/restore, trash management, tag rename/merge, orphan cleanup

---

## Phase 11: User Story 9 — Temas, mobile e PWA (Priority: P3)

**Goal**: Light/dark theme toggle (persisted), fully responsive mobile layout (touch targets >= 44px), PWA installable with offline read-only.

**Independent Test**: Toggle theme — persists after reload. Open on mobile — all UI usable with touch. Install PWA, go offline — cached documents readable.

### Implementation for User Story 9

- [X] T054 [P] [US9] Implement theme toggle in `App.svelte` — button switches `data-theme` attribute on `<html>`, persist choice in `localStorage`, CSS custom properties in `app.css` define light/dark tokens
- [X] T055 [P] [US9] Implement responsive layout in `App.svelte` and `Sidebar.svelte` — mobile: sidebar as overlay/drawer with hamburger toggle, touch targets >= 44px, content area full-width; desktop: sidebar always visible
- [X] T056 [US9] Update service worker `sw.js` for offline read-only — cache app shell (HTML, CSS, JS, icons) and recently viewed documents, serve from cache when offline, show "offline mode" banner when writes are attempted
- [X] T057 [US9] Ensure `manifest.webmanifest` is complete — name, short_name, icons (192px, 512px), start_url, display: standalone, theme_color, background_color

**Checkpoint**: Theme toggles and persists, mobile layout fully usable, PWA installable, offline read-only works

---

## Phase 12: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, Docker, CI, and final validation

- [X] T058 [P] Create C4 Level 1 (Context) documentation in `docs/c4/context.md` — system context diagram in Mermaid showing User, PKD system, external integrations (share target, public links)
- [X] T059 [P] Create C4 Level 2 (Container) documentation in `docs/c4/container.md` — Go backend, Svelte frontend, SQLite database, attachment volume
- [X] T060 [P] Create C4 Level 3 (Component) documentation in `docs/c4/component.md` — chi router, stores, handlers, middleware, sessions, security
- [X] T061 [P] Create C4 Level 4 (Code) documentation in `docs/c4/code.md` — key structs (Document, Link, Tag), handler signatures, store interfaces
- [X] T062 Update `Dockerfile` to three-stage build per research.md Decision 6 — Stage 1: `node:22-alpine` builds Svelte, Stage 2: `golang:1.25-alpine` builds Go binary with embedded frontend, Stage 3: `distroless/static-debian12` runtime
- [X] T063 Update `.github/workflows/` CI pipeline — add Node.js install + `npm ci` + `npm run build` step before Go build, ensure `internal/server/web/dist/` is populated
- [X] T064 Update `UNRAID.md` with any new environment variables or volume changes
- [ ] T065 Remove old vanilla JS frontend files from `internal/server/web/js/`, `internal/server/web/css/`, and `internal/server/web/*.html` (replaced by Svelte build output in `web/dist/`)
- [ ] T066 Run `quickstart.md` smoke test (10 steps) — validate all features end-to-end
- [ ] T067 Verify Docker image size <= 30 MB (SC-006) — build and check with `docker images`
- [ ] T068 Security review — verify CSP headers, CSRF on all mutating endpoints, HTML sanitization on capture input, no credentials in repo

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational. No other story dependencies. **This is the MVP.**
- **US2 (Phase 4)**: Depends on Foundational + US1 (needs Editor.svelte for `[[` integration)
- **US3 (Phase 5)**: Depends on Foundational + US1 (needs Sidebar.svelte for tag filtering)
- **US4 (Phase 6)**: Depends on Foundational + US2 (needs links data to visualize)
- **US5 (Phase 7)**: Depends on Foundational only (backend endpoint + PWA config, no frontend dependency)
- **US6 (Phase 8)**: Depends on Foundational + US1 (needs Editor.svelte for share button)
- **US7 (Phase 9)**: Depends on Foundational only (standalone calendar component)
- **US8 (Phase 10)**: Depends on Foundational only (standalone admin panel)
- **US9 (Phase 11)**: Depends on US1 (needs App.svelte and Sidebar.svelte established)
- **Polish (Phase 12)**: Depends on all user stories being complete

### User Story Dependencies

```
Phase 1 (Setup)
    └── Phase 2 (Foundational)
            ├── US1 (Phase 3) ← MVP
            │    ├── US2 (Phase 4) ← needs Editor
            │    │    └── US4 (Phase 6) ← needs links
            │    ├── US3 (Phase 5) ← needs Sidebar
            │    ├── US6 (Phase 8) ← needs Editor
            │    └── US9 (Phase 11) ← needs layout
            ├── US5 (Phase 7) ← independent (backend + PWA)
            ├── US7 (Phase 9) ← independent (calendar)
            └── US8 (Phase 10) ← independent (admin)
                    └── Phase 12 (Polish)
```

### Within Each User Story

- Models/stores before components
- Backend endpoints before frontend consumers
- Core implementation before integration

### Parallel Opportunities

- All Phase 1 tasks T003-T006 can run in parallel
- Phase 2 tasks T009-T011 can run in parallel (different files)
- US5, US7, US8 can run in parallel after Foundational (no cross-dependencies)
- Within US2: T025 and T026 (backend) can run in parallel with T028-T029 (frontend extensions)
- Within US8: all admin sections (T049-T053) can run in parallel
- C4 documentation (T058-T061) can all run in parallel

---

## Parallel Example: User Story 2 (Bidirectional Links)

```bash
# Backend tasks in parallel:
Task T025: "Create link store in internal/store/links.go"
Task T026: "Create link handlers in internal/server/handlers_links.go"

# Frontend extensions in parallel:
Task T028: "Build docLink TipTap extension in frontend/src/lib/editor/doclink-extension.js"
Task T029: "Build [[ autocomplete in frontend/src/lib/editor/link-suggestion.js"

# Then integration (sequential):
Task T027: "Extend PUT /api/documents/{id} for link sync"
Task T030: "Integrate extensions into Editor.svelte"
Task T031: "Create backlinks panel"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (Svelte scaffold)
2. Complete Phase 2: Foundational (DB migration, API client, routing, auth)
3. Complete Phase 3: User Story 1 (Document CRUD + rich editor)
4. **STOP and VALIDATE**: Test document creation, editing, hierarchy, image upload, auto-save
5. Deploy/demo if ready — this is a fully functional note-taking app

### Incremental Delivery

1. Setup + Foundational → Framework ready
2. Add US1 → Document CRUD (MVP!)
3. Add US2 → Bidirectional links → The PKM becomes a "second brain"
4. Add US3 → Search & filter → Retrieval pillar complete
5. Add US4 → Graph view → Connection pillar visualized
6. Add US5 → Capture → Curation pillar complete
7. Add US6 → Share → Collaboration
8. Add US7-US9 → Calendar, Admin, Themes → Polish
9. Phase 12 → Docs, Docker, CI → Ship

### Parallel Track Strategy

With the ability to run parallel tasks:

1. Complete Setup + Foundational together
2. Start US1 (blocks most others)
3. While US1 in progress, start backend-only work for US5 (T041-T042) and US7/US8 (standalone components)
4. Once US1 complete: start US2 + US3 + US6 in parallel
5. Once US2 complete: start US4 (needs links)
6. Once all stories done: Phase 12 (Polish)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable after its dependencies are met
- Tests are NOT included (not explicitly requested in spec — add `/speckit.clarify` if TDD is desired)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- The existing Go backend handlers (auth, documents, search, tags, etc.) are preserved — tasks only add NEW handlers or extend existing ones
- Total: 68 tasks across 12 phases
