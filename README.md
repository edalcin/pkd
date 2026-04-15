# PKD — Personal Knowledge Database

A self-hosted, single-user knowledge base delivered as one small Docker image (~20 MB).

**Image**: `ghcr.io/edalcin/pkd:latest`

---

## Features

- **Hierarchical documents** — unlimited nesting, drag-and-drop reorder
- **Rich text editor** — CKEditor 5 with inline resizable images, tables, code blocks
- **Hashtag tagging** — assign `#tags`, filter tree by one or more tags
- **Full-text search** — substring search across title, body, and tags (FTS5)
- **Calendar view** — browse documents by creation date
- **File attachments** — attached files live on a host-mounted volume, survive container rebuilds
- **Public share links** — revocable per-document links, read-only, stricter CSP
- **Administration** — manual backup/restore, orphan cleanup, tag rename/merge, trash management
- **Light/dark theme** — persists in `localStorage`
- **Mobile-friendly** — responsive layout, 44px touch targets
- **PWA support** — installable, read-only offline mode

---

## Quick start

```bash
docker run -d \
  --name pkd \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /path/to/pkd/db:/data/db \
  -v /path/to/pkd/attachments:/data/attachments \
  -e PKD_PASSWORD='REPLACE_WITH_A_STRONG_PASSWORD' \
  -e PKD_DB_PATH=/data/db/pkd.sqlite \
  -e PKD_ATTACHMENTS_PATH=/data/attachments \
  ghcr.io/edalcin/pkd:latest
```

Open `http://localhost:8080`, enter the master password.

**Full installation guide**: [`specs/001-personal-knowledge-db/quickstart.md`](specs/001-personal-knowledge-db/quickstart.md)

**UNRAID GUI walkthrough**: [`docs/unraid-install.md`](docs/unraid-install.md)

---

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `PKD_PASSWORD` | **yes** | — | Master password (runtime only, never stored) |
| `PKD_DB_PATH` | **yes** | — | Path to SQLite file inside container |
| `PKD_ATTACHMENTS_PATH` | **yes** | — | Path to attachments directory inside container |
| `PKD_LISTEN_ADDR` | no | `:8080` | HTTP listen address |
| `PKD_SESSION_IDLE_MINUTES` | no | `60` | Idle session timeout |
| `PKD_MAX_IMAGE_MB` | no | `10` | Max inline image upload size |
| `PKD_MAX_ATTACHMENT_MB` | no | `100` | Max file attachment size |
| `PKD_TRUST_PROXY_HEADERS` | no | `0` | Set to `1` only when behind a trusted reverse proxy |

---

## Documentation

- [Quickstart & UNRAID install](specs/001-personal-knowledge-db/quickstart.md)
- [UNRAID GUI walkthrough](docs/unraid-install.md)
- [Security reference](docs/security.md)
- [Operations guide](docs/operations.md)

---

## Building from source

Requires Go 1.23+. No CGO, no Node.js.

```bash
git clone https://github.com/edalcin/pkd.git
cd pkd
go test ./...
PKD_PASSWORD=devpassword \
PKD_DB_PATH=/tmp/pkd.sqlite \
PKD_ATTACHMENTS_PATH=/tmp/pkd-att \
go run ./cmd/pkd
```

---

## License

MIT © 2026 Eduardo Dalcin
