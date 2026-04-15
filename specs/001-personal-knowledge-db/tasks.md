---
description: "Task breakdown for feature 001-personal-knowledge-db"
---

# Tasks: Personal Knowledge Database (PKD)

**Input**: Design documents from `/specs/001-personal-knowledge-db/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml, quickstart.md

**Tests**: Included. The plan explicitly calls for unit, integration, and contract tests (`tests/unit/`, `tests/integration/`, `tests/contract/`) and a Docker smoke test in CI. Every endpoint in `contracts/openapi.yaml` must have a contract test; every user story must have an integration test covering its "Independent Test" criterion.

**Organization**: Tasks are grouped by user story so each story can be implemented and shipped independently. User Stories 1 and 2 together form the MVP (both P1).

**Branch policy reminder**: This project is **main-only**. All work commits directly to `main`. The `001-` prefix is a directory name inside `specs/`, not a git branch.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1..US9) — required on user-story phase tasks only
- Every task gives an exact file path

## Path Conventions

Paths below are all relative to the repository root (`D:\git\pkd\`). Structure per `plan.md`:

- Backend: `cmd/pkd/`, `internal/{config,server,store,model,security,sessions}/`
- Frontend (embedded via `//go:embed`): `web/{css,js,icons,vendor}/`
- Tests: `tests/{unit,integration,contract}/`
- Infrastructure: `Dockerfile`, `.github/workflows/`, `docs/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Bootstrap the Go module, dependency set, and repository hygiene files so every subsequent task has a consistent scaffold.

- [X] T001 Create the directory skeleton listed in `plan.md` (`cmd/pkd/`, `internal/{config,server,store,model,security,sessions}/`, `web/{css,js,icons,vendor/ckeditor5}/`, `tests/{unit,integration,contract}/`, `docs/`, `.github/workflows/`) with empty `.gitkeep` placeholders so the tree is committable
- [X] T002 Initialize the Go module in `go.mod` with `module github.com/edalcin/pkd` and `go 1.23`; run `go mod tidy` after the first dependency is added
- [X] T003 [P] Add runtime dependencies to `go.mod`: `github.com/go-chi/chi/v5`, `modernc.org/sqlite`, `github.com/microcosm-cc/bluemonday`, `golang.org/x/crypto` (for `argon2` + `subtle`)
- [X] T004 [P] Create `.gitignore` covering `pkd`, `pkd.exe`, `/data/`, `/tmp/`, `*.sqlite`, `*.sqlite-wal`, `*.sqlite-shm`, `.env`, and the IDE folders (`.vscode/`, `.idea/`)
- [X] T005 [P] Create `.dockerignore` that mirrors `.gitignore` plus `tests/`, `docs/`, `specs/`, `.git/`, `.github/`
- [X] T006 [P] Create `.editorconfig` with Go tab indentation (tabs, width 4), LF line endings, UTF-8, trim trailing whitespace
- [X] T007 [P] Create `LICENSE` (MIT — the public-share feature implies permissive reuse) and a minimal `README.md` that points at `specs/001-personal-knowledge-db/quickstart.md`
- [X] T008 Vendor the CKEditor 5 custom build into `web/vendor/ckeditor5/` per `research.md §4` (plugins: Image, ImageResize, ImageUpload, Table, Link, List, CodeBlock, Heading, PasteFromOffice); commit the pre-built bundle so no Node.js is needed at Docker build time

**Checkpoint**: `go build ./...` succeeds on an empty module; repository layout matches `plan.md`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Stand up the pieces that every user story depends on — env-var config, SQLite open + schema migration, HTTP server + middleware chain, security headers, CSRF, contract-test harness, and embedded-asset wiring. No user-story work can begin until this phase is complete.

**⚠️ CRITICAL**: Do not skip ahead to US1 before T020 lands — auth depends on the config + session plumbing established here.

### Configuration & entry point

- [X] T009 Implement `internal/config/config.go` with a `Load()` function that reads `PKD_PASSWORD` (required, error on empty), `PKD_DB_PATH` (required), `PKD_ATTACHMENTS_PATH` (required), `PKD_LISTEN_ADDR` (default `:8080`), `PKD_SESSION_IDLE_MINUTES` (default `60`), `PKD_MAX_IMAGE_MB` (default `10`), `PKD_MAX_ATTACHMENT_MB` (default `100`), `PKD_TRUST_PROXY_HEADERS` (default `0`), returning a typed `Config` struct; fail loudly with non-zero exit if any required var is missing
- [X] T010 Implement `cmd/pkd/main.go` with the boot sequence: load config → open DB → run migrations → build server → `ListenAndServe` with graceful shutdown on `SIGINT`/`SIGTERM`; also implement the `-healthcheck` flag (used by the Dockerfile `HEALTHCHECK`) which opens the DB read-only, verifies `SELECT 1`, and exits 0/1

### Storage layer — base

- [X] T011 Create `internal/store/schema.sql` with the full DDL from `data-model.md`: `documents`, `documents_fts` (FTS5, contentless, unicode61 remove_diacritics=2), `tags`, `document_tags`, `attachments`, `share_links`, plus indexes on `documents(parent_id)`, `documents(trashed_at)`, `documents(updated_at)`, `attachments(document_id)`, `share_links(document_id)`, `share_links(token_hash)`
- [X] T012 Implement `internal/store/migrate.go` with `//go:embed schema.sql` and an `Open(dbPath string)` function that opens via `modernc.org/sqlite`, sets `PRAGMA foreign_keys=ON`, `PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`, `PRAGMA busy_timeout=5000`, then applies `schema.sql` inside a transaction (idempotent via `CREATE TABLE IF NOT EXISTS`)
- [X] T013 [P] Implement `internal/store/tx.go` with a `WithTx(db *sql.DB, fn func(*sql.Tx) error) error` helper that begins, commits on nil, rolls back on error — used by every write path

### HTTP server — base

- [X] T014 Implement `internal/server/server.go` which takes `*Config`, `*sql.DB`, and a `*sessions.Store`, builds a `chi.Mux`, wires the middleware chain (request ID → real IP → recovery → security headers → CSRF → auth → handlers), and exposes `Handler() http.Handler` + `Close()` for graceful shutdown
- [X] T015 [P] Implement `internal/server/middleware_security.go` with middleware that sets `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Permissions-Policy: interest-cohort=()`, and two distinct CSPs: authenticated (`script-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'`) vs public share (`script-src 'none'; img-src 'self' data:; style-src 'self'; frame-ancestors 'none'`) — selected per route group
- [X] T016 [P] Implement `internal/server/middleware_csrf.go` with the double-submit cookie pattern: on GET, if no `pkd_csrf` cookie exists, set one with a 32-byte base64url random value; on mutating methods (POST/PUT/DELETE/PATCH), reject if `X-CSRF-Token` header doesn't match the cookie, returning 403
- [X] T017 [P] Implement `internal/security/csrf.go` with helpers `NewToken() string` (32 random bytes base64url) and `ConstantTimeEqual(a, b string) bool` wrapping `crypto/subtle.ConstantTimeCompare`
- [X] T018 [P] Implement `internal/server/handlers_pwa.go` which serves `GET /manifest.webmanifest` and `GET /sw.js` from the embedded `web/` filesystem with `Content-Type` and `Cache-Control: no-cache, must-revalidate` headers
- [X] T019 Implement `GET /healthz` in `internal/server/handlers_health.go` that does `SELECT 1` and returns 200 `{"status":"ok"}` or 503; also embed the root `web/` asset filesystem via `//go:embed all:web` in `internal/server/assets.go`

### Test harness — base

- [X] T020 Implement `tests/contract/openapi_test.go` that loads `specs/001-personal-knowledge-db/contracts/openapi.yaml` with `github.com/getkin/kin-openapi` (add to `go.mod`), boots the server against an in-memory SQLite (`:memory:` with `cache=shared`), and provides a `TestMain` helper + a `validateRequestResponse(t, req, res)` helper that downstream contract tests reuse

**Checkpoint**: `go test ./tests/contract/...` runs green against an empty schema, `curl http://localhost:8080/healthz` returns 200, and `curl -I http://localhost:8080/` carries the full security header set.

---

## Phase 3: User Story 1 - Private note-taking in a nested hierarchy (Priority: P1) 🎯 MVP

**Goal**: A single user unlocks the app with a master password, creates a nested tree of documents, renames/moves/deletes them, and everything persists behind the locked door.

**Independent Test**: Launch the app with `PKD_PASSWORD` set, unlock it, create a root document, nest children several levels deep, rename/move/delete them, lock the app, reopen — confirm everything persists exactly.

### Models & store (US1)

- [ ] T021 [P] [US1] Implement `internal/model/document.go` with a `Document` struct mirroring the schema (ID, ParentID, Title, BodyHTML, BodyText, Icon, Position, Version, TrashedAt, OriginalParentID, CreatedAt, UpdatedAt) + JSON tags per the OpenAPI `Document` schema
- [ ] T022 [P] [US1] Implement `internal/model/session.go` with a `Session` struct (ID, CreatedAt, LastSeenAt, IP) — in-memory only, never persisted
- [ ] T023 [US1] Implement `internal/store/documents.go` with `Create`, `GetByID`, `Update` (with version check — returns `ErrVersionConflict` when `stored.version != provided`), `Move` (rejects self-move and circular move by walking ancestors), `SoftDelete` (sets `trashed_at=NOW`, saves `original_parent_id`, sets `parent_id=NULL`), `Restore`, `ListTree` (hierarchical, excludes trashed), and `ListTrash`

### Security primitives (US1)

- [ ] T024 [P] [US1] Implement `internal/security/password.go` with `VerifyMaster(provided, configured string) bool` using `crypto/subtle.ConstantTimeCompare` over fixed-length SHA-256 digests (so timing doesn't leak length)
- [ ] T025 [P] [US1] Implement `internal/security/tokens.go` with `NewToken(n int) string` (cryptographically random base64url) and `HashSHA256(s string) []byte`
- [ ] T026 [P] [US1] Implement `internal/sessions/store.go`: in-memory `Store` with `Create(ip string) *Session`, `Get(id string) (*Session, bool)`, `Touch(id string)`, `Delete(id string)`, and a background goroutine that expires sessions idle beyond `cfg.SessionIdleMinutes`

### Throttling (US1)

- [ ] T027 [US1] Implement `internal/server/middleware_throttle.go` with a per-IP failed-auth counter backed by `sync.Map`: 5 failures in a rolling window → 30-minute lockout (returns 429 with `Retry-After`); honors `PKD_TRUST_PROXY_HEADERS` to pick real IP from `X-Forwarded-For` (rightmost untrusted hop) when enabled, else uses `RemoteAddr`

### Auth handlers (US1)

- [ ] T028 [US1] Implement `internal/server/handlers_auth.go` with `POST /api/login` (reads JSON `{password}`, runs through throttler, `VerifyMaster`, on success creates session, sets `Set-Cookie: pkd_session=<id>; HttpOnly; Secure; SameSite=Strict; Path=/`, returns 204; on failure increments throttler, returns 401) and `POST /api/logout` (deletes session, clears cookie, returns 204)
- [ ] T029 [US1] Implement `internal/server/middleware_auth.go`: extracts `pkd_session` cookie, looks up session, on hit calls `sessions.Touch` and injects `*Session` into request context; on miss, responds 401 for `/api/*` routes and redirects to `/login` for HTML routes; explicitly bypasses `/api/login`, `/login`, `/public/*`, `/healthz`, `/manifest.webmanifest`, `/sw.js`, and static asset routes

### Document handlers (US1)

- [ ] T030 [US1] Implement `internal/server/handlers_documents.go` with `POST /api/documents` (create), `GET /api/documents/{id}` (read), `PUT /api/documents/{id}` (update with version check — returns 409 + `VersionConflict` schema body on mismatch), `DELETE /api/documents/{id}` (soft delete → trash), `POST /api/documents/{id}/move` (with circular-move guard), `POST /api/documents/{id}/restore`
- [ ] T031 [US1] Implement `internal/server/handlers_tree.go` with `GET /api/tree` returning the nested `DocumentTreeNode[]` shape from the OpenAPI contract (excludes trashed; tag filter wired in US3)

### Frontend (US1)

- [ ] T032 [P] [US1] Create `web/login.html` — a minimal unbranded page with a single password input, CSRF-protected form POST to `/api/login`, and no external assets
- [ ] T033 [P] [US1] Create `web/index.html` — the authenticated SPA shell with slots for `<aside id="tree">`, `<main id="editor">`, and a top bar; loads `/css/app.css` and `/js/app.js` as modules
- [ ] T034 [P] [US1] Create `web/css/app.css` with CSS custom-property-based layout (no framework); light-theme variables defined on `:root` for now (dark theme added in US9)
- [ ] T035 [P] [US1] Create `web/js/app.js` (bootstrap: fetches `/api/tree`, mounts tree + editor, handles navigation, reads CSRF token from `pkd_csrf` cookie and sends it in `X-CSRF-Token` on every mutating fetch)
- [ ] T036 [US1] Create `web/js/tree.js` with the tree component: renders nested nodes, click-to-select, inline rename, right-click/long-press context menu (new child, rename, delete), drag-and-drop move (blocks dropping a node onto itself or a descendant — same rule enforced server-side at T030)

### Tests (US1)

- [ ] T037 [P] [US1] Contract test `tests/contract/auth_contract_test.go` validating `POST /api/login` and `POST /api/logout` request/response shapes against `openapi.yaml`, including the 401 and 429 branches
- [ ] T038 [P] [US1] Contract test `tests/contract/documents_contract_test.go` validating CRUD + move + restore + 409 `VersionConflict` branch
- [ ] T039 [P] [US1] Contract test `tests/contract/tree_contract_test.go` validating `GET /api/tree` response matches `DocumentTreeNode[]`
- [ ] T040 [P] [US1] Integration test `tests/integration/auth_test.go`: valid login → cookie set → subsequent `/api/tree` call succeeds; wrong password × 5 → 6th attempt returns 429 even with correct password
- [ ] T041 [P] [US1] Integration test `tests/integration/documents_crud_test.go`: create root doc → nest 3 levels → rename → move to new parent → soft-delete → reappears in trash → restore → reappears under original parent
- [ ] T042 [P] [US1] Integration test `tests/integration/move_circular_test.go`: attempt to move a node under its own descendant, assert 400 and that the tree is unchanged
- [ ] T043 [P] [US1] Unit test `tests/unit/throttle_test.go` for the per-IP lockout state machine (5 failures → locked → correct password still fails during window → unlocked after window expiry)
- [ ] T044 [P] [US1] Unit test `tests/unit/password_test.go` verifying constant-time compare accepts the correct password and rejects wrong inputs of every length

**Checkpoint**: User can unlock the app, create/rename/move/delete/restore a tree of documents, and the tree persists across restarts. MVP floor — though not yet shippable without US2.

---

## Phase 4: User Story 2 - Rich document editing with inline images (Priority: P1) 🎯 MVP

**Goal**: A CKEditor 5-powered editor that saves rich text, accepts pasted/uploaded images, lets the user drag-resize them, and protects against concurrent-edit overwrites.

**Independent Test**: Create a document, format text across several block types, paste or upload an image, drag its handles to resize, save, reopen, and verify every element renders identically.

### Sanitization (US2)

- [ ] T045 [US2] Implement `internal/security/sanitize.go` with two `bluemonday` policies: `EditorPolicy` (allows `h1-h6, p, ul/ol/li, table, thead/tbody/tr/td, pre, code, blockquote, a[href rel target], img[src alt width height], figure, figcaption, strong, em, u, s, hr, br, span[class]` + allow `style="width:..."` on `img` — needed for resize) and `PublicSharePolicy` (the editor policy minus any `script`/event attributes, stricter on `href` schemes — http/https/mailto only); also implement `SanitizeEditorHTML(in string) string` and `ExtractPlainText(in string) string` (strip tags for `body_text` FTS projection)
- [ ] T046 [P] [US2] Unit test `tests/unit/sanitize_test.go` — table-driven: XSS `<script>`, `onerror=`, `javascript:` href, SVG `<foreignObject>` payloads must all be stripped; formatting + images + resized widths must survive

### Image upload backend (US2)

- [ ] T047 [US2] Extend `internal/store/attachments.go` with `CreateImage(docID, filename, mime string, body io.Reader, maxBytes int64)` that writes to sharded path under `PKD_ATTACHMENTS_PATH/<xx>/<yy>/<random>` and returns an `/api/attachments/{id}` URL; reject when `Content-Length` (or streamed length) exceeds `PKD_MAX_IMAGE_MB`
- [ ] T048 [US2] Implement `internal/security/paths.go` with `SafeAttachmentPath(base, stored string) (string, error)` that rejects any `stored` containing `..`, absolute paths, or paths whose `filepath.Clean` result escapes `base` — used by every read/write of attachment files
- [ ] T049 [P] [US2] Unit test `tests/unit/paths_test.go` covering `..`, absolute paths, symlink-like sequences, and Windows-style `..\` separators — all must be rejected
- [ ] T050 [US2] Wire `POST /api/documents/{id}/attachments` in `internal/server/handlers_attachments.go` to call `CreateImage`, sanitize the returned URL, and respond with the `Attachment` schema; this endpoint is reused by US6 for non-image attachments

### Editor frontend (US2)

- [ ] T051 [US2] Create `web/js/editor.js`: initializes CKEditor 5 from `/vendor/ckeditor5/ckeditor.js` with the custom plugin set, configures a `SimpleUploadAdapter` pointed at `POST /api/documents/{id}/attachments`, debounced auto-save (every 2 s of idleness or on blur), and a save flow that sends `{version, title, body_html}` — on 409 displays the conflict dialog with "overwrite" / "reload" choices per FR-010a
- [ ] T052 [US2] Extend `PUT /api/documents/{id}` in `handlers_documents.go` (from T030) to (a) run `SanitizeEditorHTML` before storing `body_html`, (b) derive and store `body_text` via `ExtractPlainText`, (c) increment `version` atomically, (d) return the new version in the response
- [ ] T053 [P] [US2] Add the conflict-dialog UI element to `web/index.html` + styles in `web/css/app.css` (modal overlay, two action buttons)

### Tests (US2)

- [ ] T054 [P] [US2] Contract test `tests/contract/attachments_upload_contract_test.go` validating the image upload response shape and the 413 branch when the file exceeds `PKD_MAX_IMAGE_MB`
- [ ] T055 [P] [US2] Integration test `tests/integration/editor_save_test.go`: save a document with inline `<img>` + `width: 400px` style → reload → image and width survive sanitization
- [ ] T056 [P] [US2] Integration test `tests/integration/concurrency_test.go`: two clients load version 5; client A saves (→ version 6); client B's save with version 5 returns 409 + the stored version-6 document in the `VersionConflict` payload; client B retries with version 6 → success
- [ ] T057 [P] [US2] Integration test `tests/integration/xss_test.go`: attempt to save body with `<script>alert(1)</script>`, `<img onerror=...>`, and `javascript:` link → confirm stored `body_html` is cleaned and rendered page contains no executable paths

**Checkpoint**: **MVP is shippable here** — the core "private notebook with rich editor" experience works end-to-end. Stop here if you want to cut an early release.

---

## Phase 5: User Story 3 - Hashtag tagging and filtering (Priority: P2)

**Goal**: Users attach `#tag`s to documents and filter the tree by one or more tags.

**Independent Test**: Tag several documents across different tree locations, open the tag filter, select one tag, confirm only matching documents are shown regardless of where they live in the tree.

- [ ] T058 [P] [US3] Implement `internal/model/tag.go` with a `Tag` struct (ID, Name, CreatedAt, Count int)
- [ ] T059 [US3] Implement `internal/store/tags.go`: `NormalizeName(raw string) string` (lowercase, strip `#`, collapse whitespace, reject empty after normalize), `UpsertByName(tx, name) (Tag, error)`, `SetDocumentTags(tx, docID, names []string)` (diff current vs desired, insert/delete join rows), `ListWithCounts() []TagWithCount`, `RenameOrMerge(oldName, newName string)` (if `newName` exists, re-home join rows and drop the old tag; else rename in place)
- [ ] T060 [US3] Implement `handlers_tags.go` with `GET /api/tags` (returns `TagWithCount[]`) and `PUT /api/documents/{id}/tags` (body: `{tags: string[]}`)
- [ ] T061 [US3] Extend `GET /api/tree` from T031 to honor `?tag=foo&tag=bar` query (AND semantics per the contract) by joining through `document_tags`
- [ ] T062 [P] [US3] Create `web/js/tags.js`: a tag picker on the editor sidebar + a tree-filter control (multi-select chips) that re-fetches `/api/tree?tag=...`
- [ ] T063 [P] [US3] Contract test `tests/contract/tags_contract_test.go` for `GET /api/tags` and `PUT /api/documents/{id}/tags`
- [ ] T064 [P] [US3] Contract test `tests/contract/tree_filter_contract_test.go` asserting the `?tag=` query parameter is honored
- [ ] T065 [P] [US3] Integration test `tests/integration/tags_test.go`: tag 3 documents across different parents with `#alpha` and `#beta`; filter by `#alpha` → only alpha docs shown; filter by both → only docs with both; rename `#alpha` → `#gamma` → documents now carry `#gamma`

**Checkpoint**: Tagging works orthogonal to the tree; search can now be layered on top in US4.

---

## Phase 6: User Story 4 - Universal search across text and tags (Priority: P2)

**Goal**: A single search box returns documents where any substring matches the title, body plain-text, or a tag, with a snippet of match context.

**Independent Test**: Create documents containing distinct words and tags, run substring queries against each, verify matching documents appear with a context snippet.

- [ ] T066 [US4] Implement `internal/store/search.go` with `Index(tx, docID, title, bodyText, tagNames)` (writes into `documents_fts`), `Deindex(tx, docID)`, and `Search(q string, limit int) ([]SearchHit, error)` — primary path uses FTS5 `MATCH '"<q>"'` (quoted phrase for substring behavior via unicode61 + token prefix); fallback path uses `LIKE '%'||?||'%'` when the primary returns zero rows or the query has fewer than 3 characters
- [ ] T067 [US4] Wire the FTS5 index maintenance into existing store write paths: `documents.Create`, `documents.Update`, `documents.SoftDelete` (remove from index), `documents.Restore` (re-add), and `tags.SetDocumentTags` (re-index affected doc with new tag set)
- [ ] T068 [US4] Implement `handlers_search.go` with `GET /api/search?q=...` returning `SearchHit[]` (doc id, title, snippet with `<mark>` highlight from FTS5 `snippet()`)
- [ ] T069 [P] [US4] Create `web/js/search.js` with a top-bar search input; debounced 150 ms, ESC to clear, click-result to open the document with the snippet context scrolled into view
- [ ] T070 [P] [US4] Contract test `tests/contract/search_contract_test.go` validating `GET /api/search` response shape and empty-query 400
- [ ] T071 [P] [US4] Integration test `tests/integration/search_test.go`: seed 10 documents with distinct strings and tags → assert each substring query returns only the right hits → assert tag name search works too (via the index-time tag injection from T067)
- [ ] T072 [P] [US4] Performance smoke test `tests/integration/search_perf_test.go` that seeds 5,000 documents with lorem-ipsum bodies and asserts `Search(q)` returns in <200 ms p95 (SC-002); skipped by default behind a `-tags=perf` build tag to keep CI fast

**Checkpoint**: Full-text + tag search operational at scale target.

---

## Phase 7: User Story 5 - Chronological calendar browsing (Priority: P3)

**Goal**: Users see documents arranged on a month-grid by creation date.

**Independent Test**: Create documents on different dates, open the calendar, confirm each appears on the correct day and opens on click.

- [ ] T073 [US5] Implement `handlers_calendar.go` with `GET /api/calendar/{year}/{month}` returning `{day: int, documents: [{id, title, icon}]}[]` — queries `documents` for the given month using `strftime('%Y-%m', created_at) = ?`
- [ ] T074 [P] [US5] Create `web/js/calendar.js` with a month grid view, prev/next navigation, and day-cell popover listing titles
- [ ] T075 [P] [US5] Contract test `tests/contract/calendar_contract_test.go`
- [ ] T076 [P] [US5] Integration test `tests/integration/calendar_test.go`: seed documents in 3 different months, fetch each month, assert only documents for that month are returned grouped by day

**Checkpoint**: Temporal navigation works alongside the tree.

---

## Phase 8: User Story 6 - File attachments external to the application (Priority: P3)

**Goal**: Users attach arbitrary files to documents; files live on a host-mounted volume and survive container rebuilds.

**Independent Test**: Attach a file to a document, rebuild/replace the container, reopen the document, confirm the attachment is still listed and downloadable.

- [ ] T077 [P] [US6] Implement `internal/model/attachment.go` mirroring the `Attachment` schema (ID, DocumentID, OriginalName, StoredFilename, MIME, SizeBytes, CreatedAt)
- [ ] T078 [US6] Extend `internal/store/attachments.go` (from T047) with `CreateFile(docID, origName, mime string, body io.Reader, maxBytes int64)` for non-image attachments, `ListByDocument(docID)`, `GetByID(id)`, `Delete(id)` (removes row + file via `SafeAttachmentPath`), and `ListOrphanedStoredFiles()` (returns files on disk with no matching row — used by US8 cleanup)
- [ ] T079 [US6] Extend `handlers_attachments.go` (from T050) with `GET /api/attachments/{id}` (streams the file with original name in `Content-Disposition`, enforces `PKD_MAX_ATTACHMENT_MB` only on upload, not download) and `DELETE /api/attachments/{id}`
- [ ] T080 [P] [US6] Extend `web/js/editor.js` (from T051) with an "Attachments" panel that lists/uploads/downloads/deletes files for the current document
- [ ] T081 [P] [US6] Contract test `tests/contract/attachments_contract_test.go` covering upload, list, download, delete, and the 413 branch
- [ ] T082 [P] [US6] Integration test `tests/integration/attachments_test.go`: upload a file, restart the store against the same `PKD_ATTACHMENTS_PATH` (simulates container rebuild per FR-041), read it back byte-for-byte, delete it, confirm the file on disk is gone

**Checkpoint**: Files are durable across container lifecycles.

---

## Phase 9: User Story 7 - Public share links (Priority: P3)

**Goal**: Generate a revocable public URL for an individual document that renders read-only with no navigation and a stricter CSP.

**Independent Test**: Share a document, open the URL in a private window (no session), confirm read-only render, revoke, confirm the URL now 404s.

- [ ] T083 [P] [US7] Implement `internal/model/share.go` with a `ShareLink` struct (ID, DocumentID, TokenHash []byte, CreatedAt, RevokedAt)
- [ ] T084 [US7] Implement `internal/store/shares.go`: `Create(docID)` (generates 32-byte token, stores SHA-256 of token, returns plaintext once), `LookupByToken(plaintext)` (hashes and checks for active share), `Revoke(shareID)` (sets `revoked_at`), `ListByDocument(docID)`
- [ ] T085 [US7] Implement `handlers_share.go` (owner routes) with `POST /api/documents/{id}/shares` (returns `{token, url, revoke_id}` — token shown once) and `DELETE /api/documents/{id}/shares/{share_id}`
- [ ] T086 [US7] Implement the public handler `GET /public/{token}` in `handlers_share.go`: no auth middleware, no CSRF, no session cookie read, selects the share → loads document → renders `web/share.html` with sanitized body via `PublicSharePolicy` (from T045); responds 404 on revoked/missing tokens (never 401, to avoid leaking existence)
- [ ] T087 [P] [US7] Create `web/share.html` — a minimal read-only page with no navigation, no JS beyond `web/js/share-view.js`, loaded under the stricter public CSP from T015
- [ ] T088 [P] [US7] Create `web/js/share-view.js` (tiny: just handles image zoom; no editor, no fetch calls)
- [ ] T089 [P] [US7] Contract test `tests/contract/shares_contract_test.go` for create/revoke and the `GET /public/{token}` 200/404 branches
- [ ] T090 [P] [US7] Integration test `tests/integration/share_test.go`: create share → GET `/public/{token}` returns doc with a session-less client → revoke → GET `/public/{token}` returns 404 → confirm CSP response header shows `script-src 'none'` on the public route and `script-src 'self'` on the authenticated routes
- [ ] T091 [P] [US7] Unit test `tests/unit/share_token_test.go` verifying `TokenHash` storage is hashed (never plaintext), tokens are 32 random bytes base64url-encoded, and `LookupByToken` uses constant-time comparison

**Checkpoint**: Public share works with strict CSP and hashed tokens.

---

## Phase 10: User Story 8 - Administration (backup, cleanup, tag maintenance) (Priority: P3)

**Goal**: Manual backup/restore, orphan cleanup + VACUUM, hashtag rename/merge, and permanent trash emptying — all from an Administration screen.

**Independent Test**: Back up, make a change, restore, confirm state reverts. Separately, rename a hashtag in use → confirm every affected document now carries the new tag.

- [ ] T092 [US8] Implement `internal/store/backup.go` with `Backup(destPath string) error` (wraps `VACUUM INTO ?` — runs while other reads/writes are in flight thanks to WAL, produces a live-consistent snapshot) and `Restore(srcPath string) error` (closes current DB, moves the file into place atomically, reopens + re-applies `schema.sql` migration)
- [ ] T093 [US8] Implement admin handlers in `handlers_admin.go`:
  - `POST /api/admin/backup` (streams the backup file back as a download; uses a tempfile to avoid holding the whole snapshot in memory)
  - `POST /api/admin/restore` (multipart upload; requires the literal string `REPLACE` in the form field `confirm`)
  - `POST /api/admin/cleanup` (deletes files listed by `ListOrphanedStoredFiles` + runs `VACUUM`)
  - `PUT /api/admin/tags/rename` (body `{old, new}` → calls `tags.RenameOrMerge` — FR-036)
  - `GET /api/admin/trash` (lists trashed documents with `original_parent_id`)
  - `DELETE /api/admin/trash/{id}` (permanent delete of one)
  - `POST /api/admin/trash/empty` (permanent delete of all — FR-036a)
- [ ] T094 [P] [US8] Create `web/js/admin.js` with the admin UI: backup button (triggers download), restore form (file input + `REPLACE` confirmation input), cleanup button, tag rename form, trash list with per-row restore/delete + empty-trash
- [ ] T095 [P] [US8] Contract test `tests/contract/admin_contract_test.go` covering every admin endpoint, including the 400 branch on restore when `confirm` is not `REPLACE`
- [ ] T096 [P] [US8] Integration test `tests/integration/backup_restore_test.go`: seed 5 documents → backup to temp file → modify 3 documents + delete 1 → restore → confirm the 5 original documents are back byte-identical
- [ ] T097 [P] [US8] Integration test `tests/integration/admin_cleanup_test.go`: drop a stray file into `PKD_ATTACHMENTS_PATH` with no matching row → run cleanup → file is removed and `VACUUM` shrinks the DB
- [ ] T098 [P] [US8] Integration test `tests/integration/admin_trash_test.go`: soft-delete 3 docs → `GET /api/admin/trash` lists all 3 → `POST /api/admin/trash/empty` clears them → subsequent `restore` on any of them returns 404

**Checkpoint**: Operator can back up, restore, clean, and maintain tags without leaving the UI.

---

## Phase 11: User Story 9 - Visual customization: icons, themes, mobile & offline PWA (Priority: P3)

**Goal**: Per-document icons, light/dark theme toggle that persists, responsive mobile layout, and an installable PWA with read-only offline.

**Independent Test**: Pick an icon for a document and verify it appears in the tree; toggle the theme and verify it persists across reloads; open the app on a phone and install it as a PWA; verify the installed PWA opens and displays previously loaded content.

### Icons (US9)

- [ ] T099 [P] [US9] Drop the Lucide SVG icon library into `web/icons/` (per `research.md §21`) — ship ~80 curated icons as a single embedded set
- [ ] T100 [US9] Extend `handlers_documents.go` (from T030) `Update` path to accept an `icon` field (string, must match a known icon key — whitelist validated against the embedded icon set)
- [ ] T101 [P] [US9] Add an icon picker component to `web/js/editor.js` (modal grid of icons, click to select, persists on next save)

### Themes (US9)

- [ ] T102 [US9] Extend `web/css/app.css` with a `:root[data-theme="dark"]` block overriding the CSS custom properties (colors, borders); add a top-bar toggle in `web/js/app.js` that writes `data-theme` to `documentElement` and persists the choice in `localStorage`

### Mobile layout (US9)

- [ ] T103 [P] [US9] Add responsive media queries to `web/css/app.css`: under 768 px, the tree collapses into a hamburger drawer, the editor goes full-width, CKEditor toolbar wraps; touch targets ≥ 44 px

### PWA (US9)

- [ ] T104 [P] [US9] Create `web/manifest.webmanifest` with name, short_name, start_url `/`, display `standalone`, theme_color, background_color, and 192/512 icon pointers (icons added at T105)
- [ ] T105 [P] [US9] Create PWA icons in `web/icons/pwa/` at 192×192 and 512×512 PNGs
- [ ] T106 [US9] Create `web/sw.js` — hand-written service worker per `research.md §16`: pre-caches the app shell (index.html, login.html, app.css, app.js, tree.js, editor.js, search.js, ckeditor5 bundle, icons) on `install`; on `fetch`, serves static assets from cache-first with network fallback; for `GET /api/documents/{id}`, uses stale-while-revalidate with an LRU of the 100 most recently viewed; for any mutating request (POST/PUT/DELETE), attempts network first and on offline failure returns a synthetic 503 response with header `x-pkd-offline: read-only` and body `{"error":"offline — read only"}`
- [ ] T107 [US9] In `web/js/editor.js` (from T051), detect the `x-pkd-offline: read-only` header from a failed save and flip the editor into a visibly disabled read-only state with a banner — matches clarification Q1 A (read-only offline)
- [ ] T108 [P] [US9] Integration test `tests/integration/pwa_offline_test.go` (Go-side) — serves `/sw.js` and `/manifest.webmanifest`, asserts they return 200 with correct content-type, that the manifest validates against the W3C manifest shape (minimal schema check), and that `GET /sw.js` carries `Cache-Control: no-cache`
- [ ] T109 [P] [US9] Unit test `tests/unit/sw_parse_test.go` that parses `web/sw.js` as text and asserts it references every critical asset path (app shell list) — catches accidental typos that would break cache warming

**Checkpoint**: All 9 user stories are independently functional end-to-end.

---

## Phase 12: Polish & Cross-Cutting Concerns

**Purpose**: Everything needed to ship the Docker image, publish to `ghcr.io/edalcin/pkd`, and honor the `docs/` + `quickstart.md` contracts.

### Docker image

- [ ] T110 Create `Dockerfile` multi-stage: `FROM golang:1.23-alpine AS build` → `apk add git`, `go build -trimpath -ldflags='-s -w' -o /out/pkd ./cmd/pkd` → `FROM gcr.io/distroless/static-debian12:nonroot` → `COPY --from=build /out/pkd /pkd` → `USER nonroot:nonroot` → `EXPOSE 8080` → `HEALTHCHECK CMD ["/pkd","-healthcheck"]` → `ENTRYPOINT ["/pkd"]`
- [ ] T111 Verify the final image measures ≤ 30 MB (plan budget) by running `docker image ls ghcr.io/edalcin/pkd:dev` locally; if over budget, investigate with `dive`

### CI / release automation

- [ ] T112 Create `.github/workflows/build-and-publish.yml` per `research.md §19`: trigger on push to `main`, run `go vet`, `go test ./...`, `govulncheck ./...`, `docker buildx build --platform linux/amd64,linux/arm64`, Trivy scan, push to `ghcr.io/edalcin/pkd:latest` + `ghcr.io/edalcin/pkd:<sha>` using the `GITHUB_TOKEN` that the repo already has via `packages: write` permission — **never** commit a PAT
- [ ] T113 [P] Add a Docker smoke test step to the workflow: boot the just-built image with temp volumes + a dev password, `curl -I http://localhost:8080/login`, assert 200 + presence of `Content-Security-Policy` and `X-Frame-Options: DENY` headers, then `curl http://localhost:8080/healthz` expects 200

### Documentation

- [ ] T114 [P] Write `docs/unraid-install.md` — the full UNRAID GUI walkthrough with screenshots referenced from `quickstart.md §4`; placeholder image links are fine (commit the screenshots in a follow-up)
- [ ] T115 [P] Write `docs/security.md` — threat model + hardening inventory: master password + constant-time compare, per-IP lockout, session cookie flags, CSP split (SPA vs share), CSRF double-submit, FTS5 parameterization, path-traversal defense, share-token SHA-256 storage, attachment size caps, `PKD_TRUST_PROXY_HEADERS` caveat
- [ ] T116 [P] Write `docs/operations.md` — backup/restore how-to, orphan cleanup, hashtag maintenance, PWA cache invalidation, upgrade procedure (matches `quickstart.md §6`)
- [ ] T117 [P] Expand the root `README.md` — one-screen description, link to `quickstart.md`, `docs/unraid-install.md`, `docs/security.md`, `docs/operations.md`, and the ghcr.io image URL

### Final validation

- [ ] T118 Run the 10-step first-use smoke test from `quickstart.md §9` manually against a locally-built image; tick off every story it exercises (US1–US9)
- [ ] T119 Run all three test suites once more with coverage: `go test ./... -cover` — aim for ≥70 % line coverage on `internal/store`, `internal/security`, and `internal/server`
- [ ] T120 Tag the image as `latest` on GHCR by pushing a commit to `main` that bumps `README.md`'s release badge — the workflow in T112 handles the rest

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)** — no dependencies, can start immediately
- **Foundational (Phase 2)** — depends on Setup; **blocks every user story**
- **User Stories (Phase 3–11)** — all depend on Foundational; once Foundational lands, each user story phase is independent and can proceed in parallel
  - US1 (Phase 3) and US2 (Phase 4) are both P1 → together they are the MVP
  - US3 (Phase 5), US4 (Phase 6) are P2
  - US5–US9 (Phases 7–11) are P3
- **Polish (Phase 12)** — depends on whichever user stories are being shipped in the release

### User story dependencies

- **US1** — no prerequisites beyond Foundational. **Core MVP half.**
- **US2** — depends on US1 for the document CRUD pathway (extends `PUT /api/documents/{id}`) but could be developed in parallel if US1's handler signatures are agreed upfront. **Core MVP half.**
- **US3** — depends on US1 (documents must exist to tag). Independent of US2/US4.
- **US4** — depends on US1 (documents) and **weakly** on US3 (tag-aware index entries improve hit quality, but search works on title/body alone if US3 is not yet done).
- **US5** — depends only on US1.
- **US6** — depends on US1 (attachments hang off documents) and on the attachment primitives introduced in US2 (same `attachments` table + sharded storage). Treat US6 as an extension of the US2 attachment code path.
- **US7** — depends on US1 (documents) and US2 (sanitizer policies). Public CSP from Phase 2 is a prerequisite.
- **US8** — depends on every prior story that produces data it administers: documents (US1), attachments (US2/US6), tags (US3), trash (US1).
- **US9** — depends on US1 + US2 (icons picked on documents, theme wraps the editor, PWA caches the app shell which includes the CKEditor bundle).

### Within each user story

- Models → store → handlers → frontend → tests is the default inner order
- Tests marked [P] (contract + unit) can be written up-front in parallel with the implementation since they compile against a stable OpenAPI / data-model contract
- Integration tests run last within a story and double as the independent-test criterion for that story's checkpoint

### Parallel opportunities

- All `[P]` tasks within **Setup** (T003–T007) can run in one shot
- All `[P]` tasks within **Foundational** (T015–T018) can run in one shot after T014
- Within each user story, everything tagged `[P]` can run concurrently (frontend files vs backend files vs tests are in disjoint files)
- If staffed for it, Phases 3 and 4 can be developed in parallel by two people after Foundational completes; Phases 5–11 can then fan out further

---

## Parallel Example: User Story 1 (inner burst)

```bash
# After T020 (Foundational checkpoint) completes, kick these off together:
Task: "T021 [P] [US1] Create Document model in internal/model/document.go"
Task: "T022 [P] [US1] Create Session model in internal/model/session.go"
Task: "T024 [P] [US1] Implement password.go in internal/security/"
Task: "T025 [P] [US1] Implement tokens.go in internal/security/"
Task: "T026 [P] [US1] Implement sessions store in internal/sessions/store.go"
Task: "T032 [P] [US1] Create web/login.html"
Task: "T033 [P] [US1] Create web/index.html"
Task: "T034 [P] [US1] Create web/css/app.css"

# Tests in parallel with implementation (they compile against the OpenAPI contract, not runtime):
Task: "T037 [P] [US1] Contract test auth"
Task: "T038 [P] [US1] Contract test documents"
Task: "T039 [P] [US1] Contract test tree"
Task: "T043 [P] [US1] Unit test throttle"
Task: "T044 [P] [US1] Unit test password"
```

---

## Implementation Strategy

### MVP First (US1 + US2)

1. Complete **Phase 1: Setup** (T001–T008)
2. Complete **Phase 2: Foundational** (T009–T020) — critical gate
3. Complete **Phase 3: US1** (T021–T044) — nested tree + locked door
4. Complete **Phase 4: US2** (T045–T057) — rich editor + concurrency safety
5. **STOP and validate**: run the first four steps of the `quickstart.md §9` smoke test manually. If they pass, you have a shippable MVP — a single-user locked notebook with rich text and images.

### Incremental delivery (post-MVP)

- **Release 0 (MVP)**: Setup + Foundational + US1 + US2 → cut image, hand to yourself, dogfood for a week
- **Release 1**: + US3 + US4 → tag + search; the point at which PKD stops being a toy
- **Release 2**: + US5 + US6 → calendar browsing + file attachments
- **Release 3**: + US7 + US8 → share links + Administration; the point at which PKD is production-ready for daily use
- **Release 4**: + US9 → visual polish + PWA; the point at which it's usable on the phone from the couch

### Parallel team strategy (if staffed for it)

- Pair A: Setup + Foundational together
- After Foundational: Pair A owns US1; Pair B owns US2 in parallel (requires an agreed `Document` struct + handler interface contract up front, ~30 min alignment)
- For US3–US9, one developer per user story is sufficient; they diverge cleanly on different handler files and `web/js/*.js` files
- US6 should go to whoever owns US2 (shared attachment code path)

---

## Notes

- `[P]` = different files, no dependency on an incomplete task
- `[Story]` label maps every user-story task back to its row in `spec.md` for traceability
- Every task has an exact file path so a fresh contributor can pick it up cold
- Tests go in `tests/{unit,integration,contract}/` — no test files inside `internal/` (keeps the binary small and enforces that tests exercise public APIs only)
- Commit after each task or each logical group; **never** create a feature branch (main-only policy)
- When a task's code is security-sensitive (anything in `internal/security/`, `middleware_auth.go`, `middleware_throttle.go`, `middleware_csrf.go`, `handlers_share.go`, `handlers_admin.go`, `paths.go`), pair with the `security-reviewer` agent before committing
- **Stop at each checkpoint** to verify the story's Independent Test criterion from `spec.md` — that is the contract the phase must honor before moving on
- Avoid: vague tasks, same-file collisions on `[P]` items, cross-story dependencies that break a story's independent testability
