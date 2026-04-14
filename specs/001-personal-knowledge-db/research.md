# Phase 0 Research: Personal Knowledge Database (PKD)

**Feature**: `001-personal-knowledge-db`
**Date**: 2026-04-14
**Status**: Complete — no unresolved `NEEDS CLARIFICATION` items carried forward.

This document records the decisions made for each ambiguous or unknown area in the Technical Context, along with the alternatives considered and why they were rejected. The organizing principle throughout is the user's explicit directive: **prioritize simplicity and the smallest Docker image possible**. Where two options both meet the spec, the smaller / simpler one wins.

---

## 1. Backend language & runtime

**Decision**: **Go 1.23**, with CGO disabled, producing a single static binary.

**Rationale**:
- Produces a ~15–20 MB static binary that runs from `gcr.io/distroless/static-debian12:nonroot` (image ~25 MB incl. binary + embedded assets) — well under the ≤30 MB budget.
- Standard library already has a production HTTP server, `html/template`, `embed` for bundling the frontend, `crypto/rand`, `crypto/subtle`, and `database/sql`. Minimal third-party surface area reduces supply-chain risk (FR-045).
- Pure-Go SQLite driver (`modernc.org/sqlite`) exists and supports FTS5, eliminating the need for CGO and allowing `FROM scratch`-style images.
- Fast cold start (<200 ms) satisfies the container-start performance goal.
- Backend/frontend split is unnecessary: a single Go binary can serve everything including the embedded JS editor bundle.

**Alternatives considered**:
- **Rust + Axum + rusqlite**: comparable binary size, excellent safety, but slower iteration speed and CGO-equivalent build complexity for SQLite. Rejected on dev-velocity grounds for a personal project; would be revisited only if Go introduced a disqualifying limitation.
- **Python + FastAPI + SQLite**: fastest to write but alpine-python images land at 50–80 MB, roughly 2–3× the budget. Rejected on image size.
- **Node.js + Fastify + better-sqlite3**: `node:20-alpine` base plus dependencies typically lands at 100+ MB; also introduces npm supply-chain concerns. Rejected.
- **Deno / Bun**: modern single-binary runtimes, but their base images are still 50–100 MB. Rejected.

---

## 2. Database driver

**Decision**: **`modernc.org/sqlite`** (pure-Go, no CGO).

**Rationale**:
- Pure Go → no CGO → trivial cross-compilation to `linux/amd64` and `linux/arm64` and compatibility with `distroless/static` / `FROM scratch`.
- Supports **FTS5**, which is required for substring search across title/body/tags within the SC-002 latency budget.
- Actively maintained; widely used in Go projects that need self-contained binaries.

**Alternatives considered**:
- **`mattn/go-sqlite3`**: the canonical Go SQLite binding, but requires CGO → complicates cross-compilation and prevents `FROM scratch` images. Rejected.
- **Raw file-based search (no SQLite FTS)**: `LIKE '%...%'` scans over 5,000+ rows would not reliably meet SC-002. Rejected.

---

## 3. HTTP router & middleware stack

**Decision**: **`github.com/go-chi/chi/v5`** for routing + a hand-written middleware chain.

**Rationale**:
- Small, zero-dependency, stdlib-compatible `http.Handler` interface — no framework lock-in.
- Clear middleware idiom lets us stack auth, CSRF, throttling, and security headers in a readable order.
- ~3 KB of router code, negligible binary size impact.

**Alternatives considered**:
- **`net/http` stdlib only** (Go 1.22+ supports method-based routing): viable and would shave a dependency, but chi's subrouters and middleware chain produce clearer code for the 10+ endpoints this app needs. Kept as fallback if dependency minimization becomes blocking.
- **Gin, Echo, Fiber**: heavier; Fiber isn't `net/http`-compatible which complicates testing with `httptest`. Rejected.

---

## 4. Rich-text editor

**Decision**: **CKEditor 5** with a custom build including `Image`, `ImageResize`, `ImageUpload`, `ImageStyle`, `Table`, `Link`, `List`, `CodeBlock`, `Heading`, and `PasteFromOffice` plugins.

**Rationale**:
- The user explicitly referenced Trilium and asked us to "consider using the same editor". Trilium is built on CKEditor 5. This is the closest faithful match.
- CKEditor 5's `ImageResize` plugin gives exactly the "drag-to-resize inline images" behavior FR-013 requires.
- Custom builds let us drop every plugin we don't need, keeping the bundle near ~500 KB gzipped.
- Mature handling of paste-from-office, paste-image, drag-drop upload — all FR-012 requirements.
- License: GPL2+ for the open-source distribution is compatible with self-hosting and redistribution of this personal tool; we will ship the bundled build and include its `LICENSE.md` in `web/vendor/ckeditor5/` per upstream requirements.

**Alternatives considered**:
- **TipTap** (ProseMirror wrapper, MIT): smaller bundle, excellent developer ergonomics, image-resize extension exists. Runner-up. Rejected only because the user explicitly referenced Trilium/CKEditor 5. If CKEditor 5 licensing becomes problematic at implementation time, TipTap is the documented fallback.
- **Quill**: smaller but image-resize requires third-party plugins of variable quality. Rejected.
- **Trix** (Basecamp, MIT): simpler API, but image-resize is not a first-class feature. Rejected.
- **Editor.js**: block-based model that doesn't match Trilium's "continuous document" feel. Rejected.

**Action item for implementation**: verify CKEditor 5 license terms at the time of pinning the build version; if a license update requires commercial terms for any of the listed plugins, switch to TipTap and update `internal/security/sanitize.go`'s allowlist accordingly.

---

## 5. HTML sanitization (editor body + public share view)

**Decision**: **`github.com/microcosm-cc/bluemonday`** with two policies:
1. **Editor policy** — allows the subset of tags CKEditor 5 emits (headings, paragraphs, formatted text, lists, tables, links, code blocks, images with `src`/`alt`/`width`/`height`/`style` limited to a safe allowlist).
2. **Share-view policy** — stricter derivative of the editor policy that **explicitly blocks** `<script>`, `<iframe>`, `<object>`, `<embed>`, event handlers (`on*`), `javascript:` URIs, and any `style` directive except width/height on `<img>`.

**Rationale**:
- bluemonday is the de facto Go HTML sanitizer, well-audited, and supports the tag/attribute allowlist model we need for FR-042.
- Two policies let us run a slightly looser one server-side for the authenticated editor round-trip while keeping the public share view on a maximum-restriction policy. The public view is the highest-risk surface (unauthenticated, internet-reachable) and deserves the stricter policy.
- Sanitization runs on **write** (before storing) AND on **read** (before rendering into the share view) — defense in depth, per FR-042.

**Alternatives considered**:
- **Template auto-escaping only** (`html/template`): necessary but insufficient — auto-escaping does not neutralize tags already present in user-authored HTML. Kept in addition, not instead.
- **DOMPurify on the client**: client-side sanitization cannot be trusted because the server must guard against direct API calls bypassing the browser. Rejected as the primary defense.

---

## 6. Master-password handling

**Decision**: Read `PKD_PASSWORD` from the environment at startup, **keep it only in memory**, and verify submitted passwords using `crypto/subtle.ConstantTimeCompare` against the raw value.

**Rationale**:
- The master password is a deployment-time secret supplied via env var (FR-001), not a user-account password. There is no multi-user directory to attack, no offline hash extraction model to worry about: if an attacker can read the container's memory they already own the app.
- Constant-time comparison defends against timing-based oracle attacks.
- Not hashing the env var in memory avoids introducing argon2 as a dependency for a purpose it doesn't serve here. We still defend against brute force through the lockout (FR-002) and against plaintext disclosure by never logging it (FR-004).

**Alternatives considered**:
- **Hash the env var at startup with argon2id, keep only the hash in memory**: slightly reduces risk of the plaintext being swapped or core-dumped, but adds an argon2 dependency and 100 ms of startup cost for marginal benefit given the threat model above. Rejected for v1; document as a future hardening option in `docs/security.md`.
- **Require the user to provide the password hash** (argon2id) **via env var**: shifts hashing responsibility to the user's install tooling (they'd have to generate a hash before `docker run`). This harms the "UNRAID GUI-only install" UX hard. Rejected.

---

## 7. Failed-login throttling (FR-002, clarified as 5 failures → 30 min per IP)

**Decision**: **In-memory per-source-IP counter** with a `sync.Map` of `ip → { count, firstFailedAt, lockedUntil }`. Reset on successful login or after lockout expiry. No persistence — a container restart clears counters, which is acceptable because the lockout only defends against active guessing, not resumed attacks.

**Rationale**:
- Single-instance app with no horizontal scaling → no need for Redis / distributed state.
- `sync.Map` avoids lock contention for read-heavy workloads.
- The lockout satisfies SC-008 (10 attempts / hour worst case after lockout fires) and avoids Denial-of-Service against the owner because it's per-source-IP, not global.
- Reverse-proxy awareness: honor `X-Forwarded-For` **only when** the `PKD_TRUST_PROXY_HEADERS=1` env var is set, so the default setup is not spoofable.

**Alternatives considered**:
- **SQLite-backed counter**: survives restarts at the cost of more writes and a periodic cleanup job. Rejected — no benefit for this threat model.
- **Exponential backoff instead of fixed threshold**: more modern, but the clarification explicitly chose the fixed 5/30 policy. Honor the clarification.

---

## 8. Session management

**Decision**: **Server-side session table** kept in memory (`map[sessionID]sessionRecord`) guarded by a mutex. Session ID is a 32-byte `crypto/rand` token delivered as an HttpOnly, Secure, SameSite=Strict cookie. Idle timeout defaults to 60 minutes, configurable via `PKD_SESSION_IDLE_MINUTES`.

**Rationale**:
- Server-side storage lets us revoke sessions on logout without relying on cookie expiry; also avoids putting any user data (even the user ID, of which there is only one) in the cookie.
- In-memory is enough: single instance, losing sessions on restart means "log in again", which is acceptable.
- SameSite=Strict kills most CSRF vectors on its own; the double-submit CSRF token is belt-and-suspenders for browsers that don't fully enforce SameSite and for cross-origin share-link flows.

**Alternatives considered**:
- **Stateless JWT in a cookie**: would survive restarts but requires signing-key management and complicates revocation. Rejected for a personal tool.
- **Persist sessions to SQLite**: extra writes for no real benefit. Rejected.

---

## 9. CSRF protection

**Decision**: **Double-submit cookie** pattern. On login, issue a `csrf_token` cookie (non-HttpOnly, SameSite=Strict). Every state-changing request (POST/PUT/PATCH/DELETE) must include an `X-CSRF-Token` header matching the cookie; mismatched or missing → 403.

**Rationale**:
- Double-submit is stateless, simple, and complements SameSite=Strict.
- Putting the token in a header rather than a form field is idiomatic for JSON APIs and easy to consume from the SPA shell.
- The public share view never accepts state-changing requests, so CSRF doesn't apply there.

**Alternatives considered**:
- **Synchronizer token pattern (server-side storage)**: stronger but more code for identical protection in this architecture. Rejected.

---

## 10. Content Security Policy

**Decision**: Two separate CSPs, one per surface:

**Authenticated SPA** (`/`, `/index.html`, `/tree`, etc.):
```
default-src 'self';
script-src 'self';
style-src 'self' 'unsafe-inline';   # CKEditor 5 needs inline styles for image resize
img-src 'self' data: blob:;          # data:/blob: for pasted/uploaded images before POST
font-src 'self';
connect-src 'self';
frame-ancestors 'none';
base-uri 'self';
form-action 'self';
```

**Public share view** (`/public/:token`):
```
default-src 'none';
script-src 'none';                   # Public view is pure HTML — no JS needed
style-src 'self';
img-src 'self' data:;
font-src 'self';
frame-ancestors 'none';
base-uri 'self';
form-action 'none';
```

**Rationale**:
- The public view is the highest-value attack target; serving it with zero JavaScript eliminates an entire class of XSS vectors on top of the sanitizer.
- The authenticated SPA has to allow inline styles (CKEditor 5 injects them for image sizing) but can still disallow inline scripts, which is the meaningful protection.

**Alternatives considered**:
- **Single unified CSP**: forces the public view to allow whatever the editor needs. Rejected — loses the main attack-surface reduction.

---

## 11. Search implementation

**Decision**: **SQLite FTS5 virtual table** over `documents_fts(title, body_text, tags)`, kept in sync with the main `documents` table via triggers. Queries go through FTS5's `MATCH` operator with the `"..." *` prefix-token pattern for substring support; a `LIKE %query%` fallback runs in parallel for title hits on very short queries.

**Rationale**:
- FTS5 is the only way to hit the SC-002 latency target (<200 ms on 5k docs) without a heavyweight external search engine.
- Prefix-token matching covers the common "typing-as-you-go" case; LIKE fallback handles arbitrary mid-word substrings (FR-020 explicitly says *any* substring).
- Triggers keep the index fresh without application-code bookkeeping.
- `body_text` is a plain-text projection computed from the sanitized rich HTML so the index doesn't bloat with markup.

**Alternatives considered**:
- **External engine (Meilisearch, Typesense, Tantivy)**: exceeds the "no external services" constraint and bloats the image. Rejected.
- **LIKE-only**: simplest but will not reliably meet SC-002 at 5k docs. Rejected.
- **Trigram index** (via a third-party SQLite extension): would serve arbitrary substrings better than FTS5 prefix, but requires a non-default extension that doesn't travel with `modernc.org/sqlite`. Rejected. The LIKE fallback fills the gap.

---

## 12. Hashtag storage & normalization

**Decision**: A `tags` table with a unique normalized `name` column, plus a `document_tags` join table. Normalization: lowercase ASCII, strip leading `#`, allow `[a-z0-9_\-]`, max length 64. Storage is indirect (by ID) so rename/merge operations update one row in `tags` instead of touching every document.

**Rationale**:
- Makes FR-035 (rename across 1,000 documents in one operation → SC-005) a single UPDATE on the `tags` row rather than N updates.
- Merging on rename is trivial: `UPDATE document_tags SET tag_id = target WHERE tag_id = source; DELETE FROM tags WHERE id = source` inside a transaction, with a UNIQUE constraint on `(document_id, tag_id)` handling collisions.

**Alternatives considered**:
- **Inline tags in a JSON column on `documents`**: simpler at write time but every rename/merge becomes a full scan. Rejected for SC-005.

---

## 13. Backup mechanism (FR-033, FR-034, SC-004)

**Decision**: **`VACUUM INTO '<target>'`** for on-demand backups. Restore is implemented by: (1) verify uploaded file is a valid SQLite database, (2) acquire a write lock, (3) close all connections, (4) atomically rename the new file into place at `$PKD_DB_PATH`, (5) reopen the pool. Both operations run from the Administration handler with an explicit confirmation.

**Rationale**:
- `VACUUM INTO` produces a **live-consistent** snapshot without needing to stop the server — directly satisfies SC-004 ("backup taken while another document was being edited still restores successfully").
- Atomic file rename avoids any moment where the DB file is in a half-written state.

**Alternatives considered**:
- **File-copy while quiescing writes**: requires a write lock and halts the app for the duration. Rejected — `VACUUM INTO` achieves the same result without downtime.
- **SQL dump (`sqlite3 .dump`)**: textual, recoverable even with schema drift, but much larger, slower, and no longer a single-step restore. Rejected for v1; documented as a future export option in `docs/operations.md`.

---

## 14. Attachment storage layout

**Decision**: On-disk under `$PKD_ATTACHMENTS_PATH`, sharded by the first two bytes of a random stored filename: `ab/cd/abcdef1234567890…`. Metadata (original filename, size, MIME, owning document) lives in the `attachments` table. Stored filenames are **never** derived from user-supplied data; the original filename is kept in metadata for display/download only.

**Rationale**:
- Sharding prevents single-directory scalability issues on filesystems that slow down past 10k entries per directory.
- Detaching stored name from user-supplied name defuses path-traversal and filename-based injection attacks at the filesystem layer (FR-044).
- The attachments path is verified at startup (`stat`, writability probe) and every write is constrained to stay inside it via `filepath.Clean` + prefix check.

**Alternatives considered**:
- **Store attachments as SQLite BLOBs**: simpler ops, single-file backup, but explodes DB size and makes streaming downloads awkward. Rejected — the user explicitly asked attachments to live *outside* the container volume used by the DB, partly for bulk-data reasons.

---

## 15. Share link tokens

**Decision**: Generate tokens as 32 random bytes from `crypto/rand`, base64url-encoded (43 chars, no padding). **Store only the SHA-256 hash** of the token in the database. Incoming requests are looked up by hashing the path parameter and joining on the hash column.

**Rationale**:
- 32 bytes = 256 bits of entropy → collision/guess infeasible (supports SC-007).
- Storing only the hash means a database leak does not expose any live share URLs — an owner-controlled revocation list is still meaningful.
- Hash lookup on request is a single indexed query, no performance cost.

**Alternatives considered**:
- **Store tokens plaintext**: simpler but loses the "database leak doesn't leak URLs" property. Rejected.
- **JWT-signed tokens without DB storage**: revocation becomes hard. Rejected.

---

## 16. PWA & service worker strategy

**Decision**: Hand-written `sw.js` implementing:
- **Pre-cache** on install: app shell (`/`, `/login`, `/index.html`, `/css/app.css`, core JS, CKEditor 5 bundle, manifest, icons).
- **Runtime cache** (stale-while-revalidate): document GETs keyed by document ID, bounded to the last ~100 viewed documents via LRU eviction.
- **Network-first for mutations**: all POST/PUT/PATCH/DELETE bypass the cache. If offline, the service worker returns a synthetic 503 with an `x-pkd-offline: read-only` header; the editor observes this and enters the "offline — read only" state per FR-040.
- **Manifest**: `display: standalone`, light and dark theme colors, single app icon, `start_url: /`.

**Rationale**:
- Hand-written is smaller than Workbox (~1 KB vs 30+ KB) and we only need three behaviors.
- Read-only offline per clarification Q1 → A is satisfied by simply never caching mutations — no client-side write queue exists.
- Stale-while-revalidate on document GETs means recently viewed docs load instantly offline, which is the whole point of the feature.

**Alternatives considered**:
- **Workbox**: well-trodden but dwarfs our needs and bloats the bundle. Rejected.
- **No runtime cache, only app-shell precache**: loses the "read what you recently viewed" offline value. Rejected.

---

## 17. Light/dark theme implementation

**Decision**: CSS custom properties (CSS variables) scoped on `:root`, with a `data-theme="light|dark"` attribute on `<html>`. Theme toggle stores preference in `localStorage`; at first load, default to `prefers-color-scheme`. No runtime CSS swap — both themes are in the same stylesheet.

**Rationale**:
- One stylesheet, one network request, instant switching, survives reloads, respects OS preference on first visit.
- Trivial to keep both themes visually consistent in a small app.

---

## 18. Docker image strategy

**Decision**: Multi-stage `Dockerfile`:

1. **Builder stage**: `golang:1.23-alpine` → `CGO_ENABLED=0 GOFLAGS=-trimpath go build -ldflags='-s -w' -o /out/pkd ./cmd/pkd`. Dependencies fetched with `go mod download` before source copy for layer caching.
2. **Final stage**: `gcr.io/distroless/static-debian12:nonroot` → copy binary + chown. `USER nonroot:nonroot`, `EXPOSE 8080`, `ENTRYPOINT ["/pkd"]`, `HEALTHCHECK` via an in-process `/healthz` endpoint.

**Expected final image size**: ~20–25 MB (distroless/static ~2 MB + Go binary ~18 MB including embedded web/ and CKEditor 5 bundle).

**Alternatives considered**:
- **`scratch` base**: marginally smaller but lacks `/etc/passwd` for `nonroot` and CA bundle for any outbound HTTPS (even though we plan to make no outbound calls, CI smoke tests may verify that HTTPS would work if ever added). `distroless/static:nonroot` is essentially scratch + the bits you always regret not having. Rejected scratch.
- **Alpine**: 5+ MB base and a shell that we never use. Rejected on both size and attack-surface grounds.

---

## 19. CI/CD pipeline

**Decision**: GitHub Actions workflow at `.github/workflows/build-and-publish.yml` that on every push to `main`:
1. Runs `go vet ./...`, `go test ./...` (unit + integration + contract).
2. Runs `govulncheck ./...` — blocks merge on any known CVE matching module graph.
3. Builds the container for `linux/amd64` and `linux/arm64` using `docker buildx`.
4. Tags the image as `ghcr.io/edalcin/pkd:latest` and `ghcr.io/edalcin/pkd:${{ github.sha }}`.
5. Pushes to GHCR using `GITHUB_TOKEN` (no long-lived PAT).
6. Runs a Trivy scan against the built image; fails the job on Critical/High CVEs (supports SC-011).
7. On success, publishes a minimal release note referencing the SHA.

**Rationale**:
- One workflow, no branch protection gymnastics (there's only `main`).
- Trivy in CI gives SC-011 a continuous gate instead of a one-shot check.
- `GITHUB_TOKEN` + GHCR is the only supported path that doesn't require committing or manually managing credentials (FR: "no credentials in the repo").

**Alternatives considered**:
- **Manual release workflow**: defers when vulnerabilities are caught. Rejected for a security-critical app.

---

## 20. Deployment target documentation

**Decision**: Provide `docs/unraid-install.md` that walks through UNRAID's **Docker → Add Container** GUI with every field filled in (repository, network, port, two host-mounted volume fields, four environment variables: `PKD_PASSWORD`, `PKD_DB_PATH`, `PKD_ATTACHMENTS_PATH`, `PKD_LISTEN_ADDR`), plus screenshots of the form at each stage. All example passwords and paths in the docs are obviously placeholder (`YOUR_STRONG_PASSWORD_HERE`, `/mnt/user/appdata/pkd/db`, `/mnt/user/appdata/pkd/attachments`).

**Rationale**: supports SC-010 ("a new user can install via UNRAID GUI without opening a terminal").

---

## 21. Resolved spec-level open questions that did NOT become clarifications

The following points were considered during research but were resolved with reasonable defaults and documented in the spec's Assumptions section, so they did not consume a clarification slot:

- **Max image size**: 10 MB per image (`PKD_MAX_IMAGE_MB`, default 10). Above this → 413 with a clear error.
- **Max attachment size**: 100 MB per file (`PKD_MAX_ATTACHMENT_MB`, default 100). Configurable.
- **Max document body size**: 2 MB of HTML after sanitization. Large enough for real-world notes, small enough to keep FTS indexing snappy.
- **Idle session timeout**: 60 minutes default, configurable.
- **Max tree depth**: no hard limit; the UI gracefully handles arbitrary depth via horizontally scrollable breadcrumbs on mobile.
- **Icon library**: ships as vendored SVGs under `web/icons/` — a curated subset of Lucide icons (permissive license). No runtime icon download.

---

## NEEDS CLARIFICATION: none carried into Phase 1

Every `NEEDS CLARIFICATION` placeholder in the plan template has been resolved in the sections above. No open items are carried forward into data modeling or contract design.
