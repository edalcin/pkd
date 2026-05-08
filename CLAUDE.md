# pkd Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-05-01 — **v1.0 released**

## Active Technologies
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi (router), modernc.org/sqlite (database driver), bluemonday (HTML sanitizer), TipTap v2 (rich text editor), D3.js (graph visualization), Svelte 5 + Vite (frontend build) (003-pkm-refactor)
- SQLite (single file, FTS5 for full-text search, new `document_links` table for bidirectional links) (003-pkm-refactor)
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, Svelte 5 + Vite (004-document-date-association)
- SQLite — single file, ISO-8601 strings for timestamps, integer columns for partial dates (004-document-date-association)
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, TipTap v2 (editor), Svelte 5 + Vite (005-document-archiving)

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
- 005-document-archiving: Added Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, TipTap v2 (editor), Svelte 5 + Vite
- 004-document-date-association: Added Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, Svelte 5 + Vite
- 003-pkm-refactor: Added Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi (router), modernc.org/sqlite (database driver), bluemonday (HTML sanitizer), TipTap v2 (rich text editor), D3.js (graph visualization), Svelte 5 + Vite (frontend build)


<!-- MANUAL ADDITIONS START -->

## Branch Strategy

`main` is the only long-lived branch. Feature branches are allowed as **ephemeral** (hours/days max, merged via PR, then deleted). Never create long-lived environment branches — dev vs. prod separation is done via Docker tags, not code.

## Docker Tags

| Tag | Meaning | Target |
|---|---|---|
| `:edge` | Latest `main` commit | UNRAID (dev) — auto-updates |
| `:sha-abc1234` | Immutable, tied to a specific commit | Audit reference |
| `:stable` | Current production version | EC2 (prod) — manual promote |
| `:v1.2.3` | Immutable semver release | Production history |

To promote dev → prod: `git tag -a vX.Y.Z -m "..."` + `git push origin vX.Y.Z`. The `promote-to-prod.yml` workflow re-tags the image automatically (no rebuild).

<!-- MANUAL ADDITIONS END -->
