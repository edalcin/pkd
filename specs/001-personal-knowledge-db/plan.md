# Implementation Plan: Personal Knowledge Database (PKD)

**Branch**: `001-personal-knowledge-db` (directory prefix only — project policy is main-only; no git branch created)
**Date**: 2026-04-14
**Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-personal-knowledge-db/spec.md`

## Summary

A single-user, password-protected, self-hosted personal knowledge base delivered as one small Docker image published to `ghcr.io/edalcin/pkd`. Users capture notes as rich documents (text + inline resizable images) arranged in an arbitrarily nested tree, tag them with hashtags, search by substring across title/body/tags, browse by creation date in a calendar, attach files stored outside the container, and publish individual notes via revocable public share links. An Administration area handles manual backup/restore, orphan cleanup, hashtag rename/merge, and permanent trash emptying. The UI is mobile-friendly with light/dark themes and installable as a read-only-offline PWA.

**Technical approach**: one Go binary that embeds the frontend (HTML + JS + CSS + CKEditor 5 bundle + icons + service worker) and persists to a single SQLite file on a host-mounted volume. SQLite FTS5 powers substring search. Attachments live on a second host-mounted volume. Master password comes from an environment variable and is verified with constant-time comparison; successful logins issue HttpOnly+Secure session cookies. Optimistic concurrency for document saves uses a per-document version column. Backups are produced on demand via `VACUUM INTO` for a live-consistent snapshot. Container image targets ≤30 MB using a multi-stage Go build into `gcr.io/distroless/static-debian12:nonroot`.

## Technical Context

**Language/Version**: Go 1.23 (pure Go, CGO disabled)

**Primary Dependencies**:
- `modernc.org/sqlite` — pure-Go SQLite driver with FTS5 support (no CGO, enables `FROM scratch`-class images)
- `github.com/go-chi/chi/v5` — minimal HTTP router with middleware stack
- `github.com/microcosm-cc/bluemonday` — HTML sanitization for rendered document body and public share view
- `golang.org/x/crypto/argon2` — used only if we keep a hashed copy of the master password in memory (see research.md)
- Go standard library: `embed`, `net/http`, `html/template`, `crypto/rand`, `crypto/subtle`, `database/sql`
- **Frontend**: vanilla JS modules + [CKEditor 5](https://ckeditor.com/ckeditor-5/) custom build including the `Image`, `ImageResize`, `ImageUpload`, `Table`, `Link`, `List`, `CodeBlock`, `Heading`, and `PasteFromOffice` plugins (matches the editor family Trilium uses)
- **PWA**: hand-written service worker (no Workbox — keeps bundle tiny), `manifest.webmanifest`

**Storage**:
- **Primary**: SQLite 3.40+ (embedded via the pure-Go driver), single file at `$PKD_DB_PATH` on a host-mounted volume
- **Attachments**: flat on-disk storage under `$PKD_ATTACHMENTS_PATH`, sharded into `ab/cd/<stored_name>` subdirectories to avoid single-directory file-count limits
- **Search**: SQLite FTS5 virtual table indexing `title`, `body_text` (plain-text projection of the rich body), and normalized tags
- **Backups**: produced via `VACUUM INTO` into a user-chosen download path; manual only, never automatic (per clarification Q3 → A)

**Testing**:
- Go `testing` package for unit tests
- `net/http/httptest` for HTTP handler / middleware integration tests
- Table-driven tests for sanitizer, throttler, path-traversal defenses
- Contract test per public HTTP endpoint against the OpenAPI spec under `specs/001-personal-knowledge-db/contracts/openapi.yaml`
- Docker smoke test in CI: build image, boot with temp volumes, `curl` the login page, assert security headers and healthcheck

**Target Platform**: Linux Docker host (validated on UNRAID). Container base: `gcr.io/distroless/static-debian12:nonroot`. Arch: `linux/amd64` and `linux/arm64` (both common on UNRAID).

**Project Type**: single-project web application (Go backend that embeds its own static frontend). Not a "frontend + backend" split — the binary serves everything.

**Performance Goals**:
- Substring search across **5,000+ documents** returns results in **<200 ms p95** on a modest home server (supports SC-002)
- Document save (optimistic-concurrency path) completes in **<100 ms p95** for a 50 KB rich body
- Cold container start to ready-for-login in **<1 s**
- Binary memory footprint **<100 MB RSS** at steady state for a 5k-document base

**Constraints**:
- Final Docker image **≤30 MB** (distroless + static Go binary + embedded CKEditor 5 custom build ~500 KB gz)
- **No external services.** No Redis, no Postgres, no message queue, no CDN dependency. The app must run fully on a LAN with no outbound network.
- **Security-first**: CSP, CSRF, HttpOnly+Secure cookies, path-traversal hardening, parameterized queries, HTML sanitization for both editor input and share-view output. Per-IP lockout on failed auth (FR-002: 5 failures → 30 min, per clarification Q5 → C).
- **Main-only git policy** — no feature branches are ever created in this project.
- No credentials of any kind committed to the repository; all runtime configuration comes from environment variables.
- Image must build reproducibly via a GitHub Actions workflow and publish to `ghcr.io/edalcin/pkd` on every push to `main`.

**Scale/Scope**:
- Target: **1 user, 5,000–20,000 documents, <10 GB of attachments**
- ~8 HTTP endpoints for the authenticated API + 1 public share endpoint + static asset serving
- 9 user stories, 45+ functional requirements, 6 domain entities
- Personal tool — no horizontal scaling, no multi-tenant concerns

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Status**: **N/A — constitution not yet ratified.**

The file `.specify/memory/constitution.md` still contains the unfilled template with `[PRINCIPLE_1_NAME]` / `[PROJECT_NAME]` placeholders. There are therefore no ratified principles to gate this plan against.

If you later ratify the constitution, re-run `/speckit.plan` (or the Constitution Check step manually) to retroactively validate this plan against its principles. The user's project-level principles from the spec input — main-only branching, no secrets in the repo, `ghcr.io/edalcin/` publication, prioritize simplicity and image size, PWA, security hardening — are treated as hard constraints throughout this plan even without a formal constitution.

**Self-imposed gates passing on this plan:**

- [x] Single-project layout (no multi-repo complexity)
- [x] No external infrastructure dependencies at runtime
- [x] All secrets via environment variables; none committed to the repository
- [x] Main-only git policy respected (directory prefix `001-` is a spec folder name, not a branch)
- [x] Image published only to `ghcr.io/edalcin/`
- [x] PWA and mobile-friendliness first-class
- [x] Security hardening listed in FR-042..FR-045 appears in the design, not deferred

No constitutional violations to justify → **Complexity Tracking section intentionally empty**.

## Project Structure

### Documentation (this feature)

```text
specs/001-personal-knowledge-db/
├── spec.md                 # Feature specification (already written)
├── plan.md                 # This file
├── research.md             # Phase 0 output — decisions & rationale
├── data-model.md           # Phase 1 output — entities, schema, state transitions
├── quickstart.md           # Phase 1 output — getting-started + UNRAID install
├── contracts/
│   └── openapi.yaml        # Phase 1 output — HTTP API contract
├── checklists/
│   └── requirements.md     # From /speckit.specify
└── tasks.md                # Phase 2 output (/speckit.tasks — not created here)
```

### Source Code (repository root)

```text
pkd/
├── cmd/
│   └── pkd/
│       └── main.go                 # Entry point: config load, DB open, server.ListenAndServe
├── internal/
│   ├── config/
│   │   └── config.go               # Env vars: PKD_PASSWORD, PKD_DB_PATH, PKD_ATTACHMENTS_PATH,
│   │                               #           PKD_LISTEN_ADDR, PKD_SESSION_IDLE_MINUTES,
│   │                               #           PKD_MAX_IMAGE_MB, PKD_MAX_ATTACHMENT_MB
│   ├── server/
│   │   ├── server.go               # chi router, middleware chain wiring
│   │   ├── middleware_security.go  # CSP, HSTS, X-Frame-Options, Referrer-Policy
│   │   ├── middleware_auth.go      # Session cookie check → request context
│   │   ├── middleware_throttle.go  # Per-IP failed-login lockout (5 / 30 min)
│   │   ├── middleware_csrf.go      # Double-submit CSRF token
│   │   ├── handlers_auth.go        # POST /login, POST /logout
│   │   ├── handlers_documents.go   # CRUD + move + version-token optimistic concurrency
│   │   ├── handlers_tree.go        # GET /tree (hierarchical listing)
│   │   ├── handlers_tags.go        # Tag list + filter
│   │   ├── handlers_search.go      # GET /search?q=...
│   │   ├── handlers_calendar.go    # GET /calendar/:year/:month
│   │   ├── handlers_attachments.go # POST /documents/:id/attachments, GET /attachments/:id
│   │   ├── handlers_share.go       # Owner: POST /documents/:id/share, DELETE .../share/:token
│   │   │                           # Public: GET /public/:token (no auth)
│   │   ├── handlers_admin.go       # Backup, restore, cleanup, rename-tag, empty-trash
│   │   └── handlers_pwa.go         # /manifest.webmanifest, /sw.js
│   ├── store/
│   │   ├── migrate.go              # Embedded SQL migrations, applied at startup
│   │   ├── schema.sql              # DDL (embedded via //go:embed)
│   │   ├── documents.go            # CRUD, move, version bump, trash, restore
│   │   ├── tags.go                 # Normalize, rename/merge, list with counts
│   │   ├── attachments.go          # Metadata rows + file-on-disk writes
│   │   ├── shares.go               # Token hash storage, active/revoked lifecycle
│   │   ├── search.go               # FTS5 index wiring + LIKE fallback
│   │   └── backup.go               # VACUUM INTO wrapper + restore swap
│   ├── model/
│   │   ├── document.go
│   │   ├── tag.go
│   │   ├── attachment.go
│   │   ├── share.go
│   │   └── session.go
│   ├── security/
│   │   ├── password.go             # Constant-time master-password compare
│   │   ├── sanitize.go             # bluemonday policy for editor body + share view
│   │   ├── paths.go                # Attachment path-traversal guards
│   │   ├── tokens.go               # crypto/rand token generator + SHA-256 hash for storage
│   │   └── csrf.go                 # Double-submit CSRF token helpers
│   └── sessions/
│       └── store.go                # In-memory session table with idle expiry
├── web/                            # All //go:embed'd into the binary
│   ├── index.html                  # Authenticated SPA shell
│   ├── login.html                  # Password prompt
│   ├── share.html                  # Public read-only share view shell (separate CSP)
│   ├── manifest.webmanifest
│   ├── sw.js                       # Service worker: app-shell + read-only doc cache
│   ├── css/
│   │   └── app.css                 # Light + dark theme tokens
│   ├── js/
│   │   ├── app.js                  # Bootstrap, routing, theme toggle
│   │   ├── tree.js                 # Tree view, drag-and-drop move (with circular-move guard)
│   │   ├── editor.js               # CKEditor 5 init + auto-save + version-token save flow
│   │   ├── search.js
│   │   ├── calendar.js
│   │   ├── tags.js
│   │   ├── admin.js
│   │   └── share-view.js           # Public share page bootstrapper
│   ├── icons/                      # SVG icon library for document icons
│   └── vendor/
│       └── ckeditor5/              # Custom CKEditor 5 build (bundled, not fetched at runtime)
├── tests/
│   ├── unit/
│   │   ├── sanitize_test.go
│   │   ├── throttle_test.go
│   │   ├── paths_test.go
│   │   └── tokens_test.go
│   ├── integration/
│   │   ├── auth_test.go
│   │   ├── documents_crud_test.go
│   │   ├── concurrency_test.go     # Version-token conflict flow
│   │   ├── search_test.go
│   │   ├── share_test.go
│   │   ├── trash_test.go
│   │   ├── backup_restore_test.go
│   │   └── admin_test.go
│   └── contract/
│       └── openapi_test.go         # Validates handlers against contracts/openapi.yaml
├── Dockerfile                      # Multi-stage: build → distroless/static
├── .dockerignore
├── .github/
│   └── workflows/
│       └── build-and-publish.yml   # On push to main → build, test, push to ghcr.io/edalcin/pkd
├── docs/
│   ├── unraid-install.md           # "docker → add" GUI walkthrough
│   ├── security.md                 # Threat model + hardening inventory
│   └── operations.md               # Backup, restore, tag maintenance how-tos
├── CLAUDE.md                       # Agent context file (written by update-agent-context.ps1)
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

**Structure Decision**: **Single-project web application with embedded frontend.** The Go binary is the whole artifact: it serves HTTP, runs the database layer, and embeds every static asset (HTML, CSS, JS, CKEditor 5 bundle, icons, service worker, web manifest) via `//go:embed`. There is no separate frontend build server, no `npm run dev`, no backend/frontend split. This choice is dictated by the "smallest possible Docker image" and "simplicity" constraints: one binary in a distroless image is the smallest and simplest viable shape. All directories listed above are real and will be created during implementation; there is no placeholder.

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified.

**No violations.** The constitution is not yet ratified, and the plan introduces no complexity beyond what the spec explicitly requires. The table below intentionally holds no rows.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)* | — | — |
