# pkd Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-04-25 — **v1.0 released**

## Active Technologies
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi (router), modernc.org/sqlite (database driver), bluemonday (HTML sanitizer), TipTap v2 (rich text editor), D3.js (graph visualization), Svelte 5 + Vite (frontend build) (003-pkm-refactor)
- SQLite (single file, FTS5 for full-text search, new `document_links` table for bidirectional links) (003-pkm-refactor)
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, Svelte 5 + Vite (004-document-date-association)
- SQLite — single file, ISO-8601 strings for timestamps, integer columns for partial dates (004-document-date-association)

- Go 1.23 (pure Go, CGO disabled) (001-personal-knowledge-db)

## Project Structure

```text
backend/
frontend/
tests/
```

## Commands

# Add commands for Go 1.23 (pure Go, CGO disabled)

## Code Style

Go 1.23 (pure Go, CGO disabled): Follow standard conventions

## Recent Changes
- 004-document-date-association: Added Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, Svelte 5 + Vite
- 003-pkm-refactor: Added Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi (router), modernc.org/sqlite (database driver), bluemonday (HTML sanitizer), TipTap v2 (rich text editor), D3.js (graph visualization), Svelte 5 + Vite (frontend build)

- 001-personal-knowledge-db: Added Go 1.23 (pure Go, CGO disabled)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
