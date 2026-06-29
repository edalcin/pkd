# pkd Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-05-18

## Active Technologies
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi (router), modernc.org/sqlite (database driver), bluemonday (HTML sanitizer), TipTap v2 (rich text editor), D3.js (graph visualization), Svelte 5 + Vite (frontend build) (003-pkm-refactor)
- SQLite (single file, FTS5 for full-text search, new `document_links` table for bidirectional links) (003-pkm-refactor)
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, Svelte 5 + Vite (004-document-date-association)
- SQLite — single file, ISO-8601 strings for timestamps, integer columns for partial dates (004-document-date-association)
- Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend) + chi v5 (router), modernc.org/sqlite v1.48.2, TipTap v2 (editor), Svelte 5 + Vite (005-document-archiving)
- Go 1.25, aws-sdk-go-v2 service/s3 v1.101.0, **aws-sdk-go-v2 feature/s3/manager (novo)**, archive/zip stdlib (ZIP64), crypto/sha256 stdlib, github.com/google/uuid v1.6.0 (005-s3-attachments-backup)
- SQLite — novo índice `idx_attachments_content_sha256`; prefixo S3 reservado `_backup-tmp/` (005-s3-attachments-backup)

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
- 005-s3-attachments-backup: Added aws-sdk-go-v2 feature/s3/manager para streaming multipart; novo pacote `internal/backup/` (manifest/writer/reader/sweep); novo `internal/server/jobs.go` (in-memory job tracking); prefixo S3 `_backup-tmp/` reservado para artefatos transitórios; índice `idx_attachments_content_sha256`
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

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
