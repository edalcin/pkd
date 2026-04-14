# Quickstart: Personal Knowledge Database (PKD)

**Feature**: `001-personal-knowledge-db`
**Audience**: the project owner running PKD for personal use, plus future contributors who need to boot the project locally.
**Goal**: go from "empty machine" to "unlocking your knowledge base at `http://localhost:8080`" in under 10 minutes.

> Every password, path, and token in this document is a **placeholder**. Never copy them verbatim into production. Never commit real credentials to this repository.

---

## 1. What you need before you start

| Need | Why | How to check |
|---|---|---|
| Docker 24+ (or UNRAID 6.12+) | The app is shipped as a single container image | `docker --version` |
| A place to keep the SQLite file | The DB lives on your host, not inside the container | Create `/path/to/pkd/db/` (will be mounted) |
| A place to keep attachments | Same idea for uploaded files | Create `/path/to/pkd/attachments/` |
| A strong master password | Gatekeeps the entire knowledge base | Generate one with `openssl rand -base64 24` |
| (Optional) A reverse proxy with TLS | Share links become public URLs — they should be HTTPS | Caddy / Traefik / nginx / UNRAID's SWAG all work |

---

## 2. Run with `docker run` (local / non-UNRAID)

```bash
# Prepare host directories (will be bind-mounted)
mkdir -p /path/to/pkd/db /path/to/pkd/attachments

# Pull the latest published image
docker pull ghcr.io/edalcin/pkd:latest

# Start the container
docker run -d \
  --name pkd \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /path/to/pkd/db:/data/db \
  -v /path/to/pkd/attachments:/data/attachments \
  -e PKD_PASSWORD='REPLACE_WITH_A_STRONG_PASSWORD' \
  -e PKD_DB_PATH=/data/db/pkd.sqlite \
  -e PKD_ATTACHMENTS_PATH=/data/attachments \
  -e PKD_LISTEN_ADDR=:8080 \
  ghcr.io/edalcin/pkd:latest
```

Open `http://localhost:8080`, enter the master password, and you're in.

**What to check on first run:**
1. `docker logs pkd` shows `listening on :8080` and `schema ready`.
2. The file `/path/to/pkd/db/pkd.sqlite` exists after the first save.
3. Uploading an image to a note creates a file under `/path/to/pkd/attachments/ab/cd/...`.
4. The login page responds with the `Content-Security-Policy` and `X-Frame-Options: DENY` headers (use `curl -I http://localhost:8080/login`).

---

## 3. Run with `docker compose`

`docker-compose.yml` (example — adjust paths and password, and keep this file out of the repo or behind `.gitignore` if it ever contains real secrets):

```yaml
services:
  pkd:
    image: ghcr.io/edalcin/pkd:latest
    container_name: pkd
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      PKD_PASSWORD: ${PKD_PASSWORD:?PKD_PASSWORD is required}
      PKD_DB_PATH: /data/db/pkd.sqlite
      PKD_ATTACHMENTS_PATH: /data/attachments
      PKD_LISTEN_ADDR: ":8080"
      # Optional tuning
      PKD_SESSION_IDLE_MINUTES: "60"
      PKD_MAX_IMAGE_MB: "10"
      PKD_MAX_ATTACHMENT_MB: "100"
    volumes:
      - ./data/db:/data/db
      - ./data/attachments:/data/attachments
    healthcheck:
      test: ["CMD", "/pkd", "-healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
```

Start with:

```bash
export PKD_PASSWORD='REPLACE_WITH_A_STRONG_PASSWORD'
docker compose up -d
```

---

## 4. Install on UNRAID via the graphical "Docker → Add Container" form

This is the installation path the product explicitly targets (SC-010). You should not need to open a terminal for any step here.

1. **Prepare the host paths** in the UNRAID file manager:
   - `/mnt/user/appdata/pkd/db`
   - `/mnt/user/appdata/pkd/attachments`

2. **Docker tab → Add Container**. Fill in:

   | Field | Value |
   |---|---|
   | Name | `pkd` |
   | Repository | `ghcr.io/edalcin/pkd:latest` |
   | Network Type | `Bridge` |
   | WebUI | `http://[IP]:[PORT:8080]` |
   | Icon URL | *(optional — the project repo provides `docs/assets/pkd-icon.png`)* |

3. **Add Port**:
   - Name: `WebUI`
   - Container Port: `8080`
   - Host Port: `8080` (change if 8080 is taken)
   - Connection Type: `TCP`

4. **Add Path** (database volume):
   - Name: `DB`
   - Container Path: `/data/db`
   - Host Path: `/mnt/user/appdata/pkd/db`
   - Access Mode: `Read/Write`

5. **Add Path** (attachments volume):
   - Name: `Attachments`
   - Container Path: `/data/attachments`
   - Host Path: `/mnt/user/appdata/pkd/attachments`
   - Access Mode: `Read/Write`

6. **Add Variable**:
   - Name: `Master Password`
   - Key: `PKD_PASSWORD`
   - Value: a strong password you generated separately — **do not reuse an existing password**
   - Type: `Password` (UNRAID will mask the field)

7. **Add Variable**:
   - Key: `PKD_DB_PATH`
   - Value: `/data/db/pkd.sqlite`

8. **Add Variable**:
   - Key: `PKD_ATTACHMENTS_PATH`
   - Value: `/data/attachments`

9. **Add Variable**:
   - Key: `PKD_LISTEN_ADDR`
   - Value: `:8080`

10. Click **Apply**. UNRAID pulls the image, starts the container, and the WebUI link appears. Click it, enter the master password from step 6, and you're in.

**If something goes wrong:** UNRAID's container log for `pkd` will include the startup line. Common first-run issues are permissions on the mounted directories (they must be writable by the nonroot user inside the container) and typos in the paths you entered in steps 4 and 5.

Full walkthrough with screenshots lives at `docs/unraid-install.md` in the repository.

---

## 5. Put it behind HTTPS (recommended for share links)

The share-link feature generates URLs anyone can open. Those URLs should be HTTPS, which means PKD should sit behind a reverse proxy that terminates TLS. PKD itself does not handle certificates (on purpose — one less moving part).

Minimal Caddy example:

```caddy
pkd.example.lan {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

Minimal UNRAID SWAG: add a subdomain proxy conf that forwards to the `pkd` container name on port 8080.

When behind a reverse proxy, set `PKD_TRUST_PROXY_HEADERS=1` so the failed-login lockout uses the real client IP from `X-Forwarded-For` instead of the proxy's IP. **Do not set this without a proxy in front**, or anyone can spoof their source IP and bypass the lockout.

---

## 6. Day-2 operations

### 6.1 Backups

Backups are **manual only** (see spec clarification Q3).

- From the UI: **Administration → Backup now**. Your browser downloads a `.sqlite` file which is a live-consistent snapshot produced via `VACUUM INTO`.
- From the host: you can also copy `/path/to/pkd/db/pkd.sqlite` and (if you care about attachments) the whole `/path/to/pkd/attachments/` tree. This works because SQLite uses WAL mode — the copy is safe even while writes are in flight, although using the in-app backup is still recommended because it is atomic.
- **Restore**: **Administration → Restore from backup**, upload the file, type `REPLACE` in the confirmation box. The DB is swapped in place and you'll be asked to log in again.

### 6.2 Emptying the trash

Deleted documents go to Trash and stay there **forever** until you explicitly empty it (see spec clarification Q2).

- **Administration → Trash**: shows every trashed document. Restore them individually, permanently delete them individually, or use **Empty Trash** for the whole set.

### 6.3 Renaming / merging a hashtag

- **Administration → Tags**: pick the tag to rename, enter the new name, confirm. If the new name already exists, the two are merged and every document touched once.

### 6.4 Cleanup

- **Administration → Cleanup**: removes orphaned attachment files (files on disk no longer referenced by any document) and runs `VACUUM` on the database. Safe to run any time.

### 6.5 Upgrading to a new image version

```bash
docker pull ghcr.io/edalcin/pkd:latest
docker stop pkd && docker rm pkd
# Re-run the same 'docker run' command from step 2
```

Because both the database and attachments live on host-mounted volumes (FR-041), nothing is lost during image rebuilds (SC-009).

---

## 7. Running the project from source (contributor path)

```bash
git clone https://github.com/edalcin/pkd.git
cd pkd

# Go 1.23+
go test ./...

# Run locally against a throwaway SQLite file
PKD_PASSWORD='devpassword' \
PKD_DB_PATH=/tmp/pkd-dev.sqlite \
PKD_ATTACHMENTS_PATH=/tmp/pkd-dev-attachments \
PKD_LISTEN_ADDR=:8080 \
go run ./cmd/pkd

# Build a production binary
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o pkd ./cmd/pkd

# Build the Docker image locally
docker build -t pkd:dev .
docker run --rm -p 8080:8080 \
  -e PKD_PASSWORD=devpassword \
  -e PKD_DB_PATH=/tmp/pkd.sqlite \
  -e PKD_ATTACHMENTS_PATH=/tmp/attachments \
  pkd:dev
```

**Remember the main-only rule**: contributions commit directly to `main`. Feature branches are not used in this project.

---

## 8. Environment variable reference

| Variable | Required? | Default | Meaning |
|---|---|---|---|
| `PKD_PASSWORD` | **yes** | — | Master password. Supplied only at runtime via env var (FR-001). |
| `PKD_DB_PATH` | **yes** | — | Absolute path inside the container to the SQLite file. Must live on a mounted volume. |
| `PKD_ATTACHMENTS_PATH` | **yes** | — | Absolute path inside the container to the attachments directory. Must live on a mounted volume. |
| `PKD_LISTEN_ADDR` | no | `:8080` | Listen address for the HTTP server. |
| `PKD_SESSION_IDLE_MINUTES` | no | `60` | Idle session timeout (FR-003). |
| `PKD_MAX_IMAGE_MB` | no | `10` | Maximum accepted size of an inline image (FR-014). |
| `PKD_MAX_ATTACHMENT_MB` | no | `100` | Maximum accepted size of a file attachment. |
| `PKD_TRUST_PROXY_HEADERS` | no | `0` | Set to `1` **only** when running behind a trusted reverse proxy. |

---

## 9. First-use smoke test

After unlocking, in order, confirm:

1. **Create a document** under the root. Give it a title. Type some content. Save. Reload the page — the content survives (covers US1 + US2).
2. **Paste an image** into the body. Drag its corner to resize it. Save. Reload. The image and its size persist (covers FR-011 through FR-014).
3. **Add a hashtag** `#smoketest`. Filter the tree by that tag — only this document appears (covers US3).
4. **Type in the search box** a substring of the title and of the body. Both find the document (covers US4).
5. **Create a child document** under the first one. Drag it to the root. It moves, and its own content is intact (covers FR-007).
6. **Generate a share link** on the document. Open it in a private browser window. Confirm it renders in read-only mode and has no navigation (covers US7 + FR-030 + FR-032). **Revoke the link**. Confirm the URL now returns 404.
7. **Backup** from Administration. Make a visible change in the document. **Restore** the backup. The visible change is gone (covers US8 + SC-004).
8. **Delete the document**. It disappears from the tree. Open **Administration → Trash** — it's there. Restore it. It reappears under its original parent (covers FR-008).
9. **Open the app on a phone**. Confirm the tree, editor, and search all work with touch (covers US9 + FR-039).
10. **Install the app as a PWA** from the phone browser menu. Launch the PWA icon. Confirm it opens standalone (covers FR-040). Turn off Wi-Fi — confirm you can still read the documents you've loaded but the editor shows "offline — read only" (covers clarification Q1 → A).

If all ten pass, the core product is working end to end.
