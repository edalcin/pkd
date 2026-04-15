# PKD — Personal Knowledge Database

A self-hosted, single-user knowledge base delivered as one small Docker image.

- Hierarchical documents (unlimited nesting)
- Rich text editor with inline, resizable images (CKEditor 5)
- Hashtag tagging + full-text search
- Calendar browsing, file attachments, public share links
- Administration: backup/restore, cleanup, tag rename
- Light/dark theme, mobile-friendly, installable as a PWA
- Runs on UNRAID, any Linux Docker host, or from source

**Image**: `ghcr.io/edalcin/pkd:latest`

## Quick start

See [`specs/001-personal-knowledge-db/quickstart.md`](specs/001-personal-knowledge-db/quickstart.md) for:

- `docker run` one-liner
- `docker compose` example
- UNRAID graphical install (Docker → Add Container)
- Environment variable reference
- First-use smoke test

## Building from source

```bash
git clone https://github.com/edalcin/pkd.git
cd pkd
go test ./...
PKD_PASSWORD=devpassword PKD_DB_PATH=/tmp/pkd.sqlite PKD_ATTACHMENTS_PATH=/tmp/pkd-att go run ./cmd/pkd
```

Requires Go 1.23+. No CGO, no Node.js build step needed.

## License

MIT
