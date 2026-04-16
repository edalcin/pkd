# Implementation Plan: PKM Refactor

**Branch**: `main` | **Date**: 2026-04-16 | **Spec**: `specs/003-pkm-refactor/spec.md`
**Input**: Feature specification from `/specs/003-pkm-refactor/spec.md`

## Summary

Refactor PKD from a simple note-taking tool into a Personal Knowledge Management (PKM) system. The three pillars driving the design are Curation (capture and contextualize information), Connection (bidirectional links and graph visualization), and Retrieval (powerful search and navigation).

**Technical approach**: Partial rewrite. The Go backend (chi router, SQLite via modernc.org/sqlite, auth, security middleware) is preserved and extended with new endpoints for links, graph data, and external capture. The vanilla JS frontend is replaced entirely by a Svelte application built with Vite, with output embedded into the Go binary via `//go:embed`. D3.js provides client-side force-directed graph rendering. C4 Model documentation with Mermaid diagrams is added to `docs/c4/`.

## Technical Context

**Language/Version**: Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend)
**Primary Dependencies**: chi (router), modernc.org/sqlite (database driver), bluemonday (HTML sanitizer), TipTap v2 (rich text editor), D3.js (graph visualization), Svelte 5 + Vite (frontend build)
**Storage**: SQLite (single file, FTS5 for full-text search, new `document_links` table for bidirectional links)
**Testing**: Go `testing` package (unit + integration + contract tests), Vitest for frontend unit tests
**Target Platform**: Docker (multi-stage: Node.js for frontend build, golang:1.25-alpine for backend build, distroless/static for runtime), UNRAID compatible
**Project Type**: Web application (single-binary server with embedded SPA)
**Performance Goals**: Document CRUD < 5s, search 5000+ docs < 1s, graph 500 nodes + 2000 edges < 3s, backup 10k docs < 10s, bidirectional link update < 100ms, Docker image <= 30 MB
**Constraints**: Single-user (password via env var), main-only git policy, no credentials in repo, all data on external volumes, Svelte build output ~50-80 KB gzipped, PWA offline read-only only, external capture online-only (no offline queue)
**Scale/Scope**: Single user, targeting 5000-10000 documents, 2000+ links, single SQLite database file

## Constitution Check

N/A -- project constitution not yet ratified.

## Project Structure

### Documentation (this feature)

```text
specs/003-pkm-refactor/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
pkd/
├── cmd/pkd/main.go                    # Entry point (existing, unchanged)
├── internal/
│   ├── config/                        # Env var config (existing, unchanged)
│   ├── server/
│   │   ├── server.go                  # Chi router (existing, new routes added)
│   │   ├── middleware_auth.go         # Auth middleware (existing)
│   │   ├── middleware_csrf.go         # CSRF double-submit (existing)
│   │   ├── middleware_security.go     # CSP/HSTS/X-Frame (existing)
│   │   ├── middleware_throttle.go     # Per-IP throttle (existing)
│   │   ├── handlers_documents.go     # Document CRUD (existing, evolved)
│   │   ├── handlers_auth.go          # Login/logout (existing)
│   │   ├── handlers_search.go        # FTS5 search (existing)
│   │   ├── handlers_tags.go          # Tag CRUD (existing)
│   │   ├── handlers_attachments.go   # File uploads (existing)
│   │   ├── handlers_share.go         # Public share links (existing)
│   │   ├── handlers_admin.go         # Backup/restore/cleanup (existing)
│   │   ├── handlers_calendar.go      # Calendar view data (existing)
│   │   ├── handlers_tree.go          # Document tree (existing)
│   │   ├── handlers_health.go        # Health check (existing)
│   │   ├── handlers_pwa.go           # PWA manifest/SW (existing)
│   │   ├── handlers_links.go         # NEW: link CRUD + backlinks API
│   │   ├── handlers_graph.go         # NEW: graph data endpoint
│   │   ├── handlers_capture.go       # NEW: external capture endpoint
│   │   ├── assets.go                 # //go:embed all:web (existing)
│   │   └── web/                      # Svelte build output (REPLACES current vanilla JS)
│   ├── store/
│   │   ├── documents.go              # Document CRUD (existing, evolved)
│   │   ├── links.go                  # NEW: document_links CRUD + backlink queries
│   │   ├── tags.go                   # Tag operations (existing)
│   │   ├── search.go                 # FTS5 search (existing)
│   │   ├── shares.go                 # Share link operations (existing)
│   │   ├── attachments.go            # Attachment metadata (existing)
│   │   ├── backup.go                 # Backup/restore (existing)
│   │   ├── tx.go                     # Transaction helpers (existing)
│   │   ├── schema.sql                # DDL (existing, document_links table added)
│   │   └── migrate.go               # Schema migrations (existing, new migration added)
│   ├── model/                         # Go structs (existing, Link model added)
│   ├── security/                      # Sanitizer, path validation, tokens (existing)
│   └── sessions/                      # In-memory sessions (existing)
├── frontend/                          # NEW: Svelte source
│   ├── package.json
│   ├── vite.config.js
│   ├── src/
│   │   ├── App.svelte                # Main layout (sidebar + content area)
│   │   ├── main.js                   # Svelte mount point
│   │   ├── lib/
│   │   │   ├── api.js                # Fetch wrapper with CSRF token handling
│   │   │   ├── stores/               # Svelte stores (documents, tags, search, etc.)
│   │   │   └── components/
│   │   │       ├── Sidebar.svelte    # Document tree + tag filter
│   │   │       ├── Editor.svelte     # TipTap v2 wrapper with [[link]] autocomplete
│   │   │       ├── GraphView.svelte  # D3.js force-directed graph
│   │   │       ├── Search.svelte     # Universal search with snippets
│   │   │       ├── Calendar.svelte   # Calendar view
│   │   │       ├── Admin.svelte      # Administration panel
│   │   │       ├── ShareView.svelte  # Public share (separate entry point)
│   │   │       └── LoginPage.svelte  # Login form
│   │   └── styles/
│   │       └── app.css               # CSS custom properties (light/dark themes)
│   └── public/
│       └── (static assets: icons, manifest)
├── docs/
│   ├── c4/                            # NEW: C4 Model documentation
│   │   ├── context.md                 # Level 1: System Context
│   │   ├── container.md               # Level 2: Container
│   │   ├── component.md               # Level 3: Component
│   │   └── code.md                    # Level 4: Code
│   ├── security.md                    # Existing
│   ├── operations.md                  # Existing
│   └── unraid-install.md             # Existing
├── Dockerfile                         # UPDATED: Node.js stage for frontend + Go stage
├── .github/workflows/                 # Existing CI (updated for frontend build)
├── UNRAID.md                          # Existing
├── README.md                          # Existing
├── go.mod / go.sum                    # Existing
└── specs/                             # SpecKit specs
```

**Structure Decision**: This follows the existing Go backend layout under `internal/` (config, server, store, model, security, sessions) and adds a `frontend/` directory at the repository root for the Svelte source. The Svelte build output replaces the current vanilla JS/HTML files in `internal/server/web/`. The Go binary continues to embed the `web/` directory via `//go:embed`. The Dockerfile gains a Node.js build stage that runs `npm run build` before the Go compilation stage. C4 documentation goes in `docs/c4/` alongside existing operational docs.

## Complexity Tracking

No violations to justify. The plan follows the existing single-binary architecture, adds one new database table (`document_links`), three new handler files, one new store file, and replaces the frontend framework. No new external services, no new databases, no new deployment targets.
