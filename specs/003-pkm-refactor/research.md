# Phase 0 Research: PKM Refactor (003-pkm-refactor)

**Date**: 2026-04-16
**Status**: Approved

This document records technology decisions for the PKM refactor. Each section covers one decision with rationale and alternatives considered.

---

## Decision 1: Svelte as frontend framework

**Decision**: Use Svelte 5 (with Vite as build tool) to rewrite the frontend.

**Rationale**: The current frontend is vanilla JS files (`tree.js`, `admin.js`, `calendar.js`, `search.js`, `tags.js`) served statically from `internal/server/web/js/`. This approach does not scale to the complexity required by graph view, bidirectional links with autocompletion, and a richer UI.

Svelte compiles to vanilla JS at build time -- no runtime library ships to the browser. This produces the smallest bundle of any major framework. A typical Svelte app ships ~15 KB gzipped of framework overhead, compared to ~45 KB for React and ~33 KB for Vue. Since the spec mandates a Docker image under 30 MB (SC-006), every kilobyte matters.

Svelte's reactivity model (`$state`, `$derived` in Svelte 5) is compiler-driven, not runtime-driven. There is no virtual DOM diffing. This is a good fit for the graph view (D3.js manipulates the DOM directly) because there is no framework layer fighting for control of DOM nodes.

Vite is the default build tool for Svelte. It provides fast HMR during development and Rollup-based production builds with tree-shaking, code splitting, and asset hashing.

**Alternatives considered**:

| Framework | Bundle (gzipped) | Trade-off |
|-----------|-------------------|-----------|
| React 19  | ~45 KB            | Largest ecosystem, but heaviest runtime. Virtual DOM conflicts with D3's direct DOM manipulation. |
| Vue 3     | ~33 KB            | Good middle ground, but Svelte is smaller and the PKD codebase is small enough that Vue's ecosystem advantage is not needed. |
| SolidJS   | ~7 KB             | Smallest runtime, but smaller ecosystem than Svelte. TipTap integration is less documented. |
| Vanilla JS (status quo) | 0 KB | No framework overhead, but the current files already show duplication and lack of component reuse. Adding graph view and link autocompletion to vanilla JS would result in unmaintainable spaghetti. |

---

## Decision 2: Svelte + TipTap integration

**Decision**: Use the `svelte-tiptap` package to wrap TipTap v2 inside a Svelte component.

**Rationale**: TipTap v2 is already chosen (spec 002) and a 402 KB pre-built bundle exists at `internal/server/web/vendor/tiptap/tiptap.min.js`. The refactor switches from loading TipTap as a global script to importing it as an ES module via npm, which enables tree-shaking (only the extensions actually used are bundled).

`svelte-tiptap` provides a thin Svelte wrapper around TipTap's `Editor` class. It exposes the editor instance as a Svelte store, making it reactive. The package is ~3 KB and delegates all real work to `@tiptap/core`.

The integration pattern:

```svelte
<script>
  import { createEditor, EditorContent } from 'svelte-tiptap';
  import StarterKit from '@tiptap/starter-kit';
  import Image from '@tiptap/extension-image';

  const editor = createEditor({
    extensions: [StarterKit, Image],
    content: '',
  });
</script>

<EditorContent editor={$editor} />
```

This replaces the current approach where `tiptap.min.js` is loaded as a `<script>` tag and the editor is initialized imperatively in vanilla JS.

**Alternatives considered**:

- **Manual wrapper (no svelte-tiptap)**: Create a Svelte component that calls `new Editor()` in `onMount` and destroys it in `onDestroy`. This works but requires manual subscription to editor events for reactivity. `svelte-tiptap` already handles this correctly. The maintenance burden of a manual wrapper is not justified for the ~3 KB saved.
- **Keep the pre-built bundle as a global script**: Incompatible with Vite's module-based build. The pre-built bundle includes all CKEditor-era extensions and cannot be tree-shaken.

---

## Decision 3: D3.js for graph view

**Decision**: Use D3.js `d3-force` module for the force-directed graph, rendered as SVG, integrated into Svelte via a dedicated component.

**Rationale**: The spec requires an interactive graph with zoom, pan, click-to-navigate, and tag-based coloring (FR-020 through FR-023). D3's force simulation (`d3-force`) is the most battle-tested force-directed layout library. It is modular -- only `d3-force`, `d3-selection`, `d3-zoom`, and `d3-drag` need to be imported (~30 KB gzipped total, not the full 80 KB D3 bundle).

SVG rendering is chosen over Canvas because:
- SVG nodes are DOM elements, so click handlers and CSS styling work natively.
- At 500 nodes / 2,000 edges (SC-003 target), SVG performs well. Canvas only becomes necessary above ~5,000 nodes.
- SVG integrates cleanly with Svelte's DOM model. Canvas would require a separate render loop.

Integration with Svelte: D3 handles the force simulation (tick calculations) and zoom/drag behaviors. Svelte handles rendering the SVG elements reactively. This avoids the common D3+framework conflict where both try to own the DOM. The pattern is:

1. D3 `forceSimulation` runs in a `$effect` and updates `nodes` and `links` arrays (plain data, not DOM).
2. Svelte's `{#each}` renders `<circle>` and `<line>` elements from those arrays.
3. D3 `zoom` and `drag` behaviors are attached to the SVG element via Svelte `use:` actions.

Performance at 500 nodes: D3 force simulation converges in ~300 ticks. At 500 nodes with 2,000 links, each tick takes ~1-2 ms on modern hardware. Total simulation time is under 500 ms. SVG rendering of 500 circles and 2,000 lines is well within browser limits. The SC-003 target of 3 seconds is achievable with margin.

**Alternatives considered**:

| Option | Trade-off |
|--------|-----------|
| Cytoscape.js | Full graph library (~170 KB gzipped). More features out-of-the-box (layouts, styles), but heavier and less control over rendering. Overkill for the simple force-directed layout needed here. |
| vis-network | ~110 KB gzipped. Canvas-based. Good for large graphs but harder to style and integrate with Svelte's reactive DOM. |
| Sigma.js | WebGL-based. Designed for 10,000+ nodes. Too heavy for 500-node use case. |
| force-graph (wrapper) | Thin wrapper over D3 force + Canvas/WebGL. Reduces boilerplate but adds a dependency for something achievable with ~100 lines of D3 code. |

---

## Decision 4: Bidirectional links data model

**Decision**: Add a `document_links` table with `(source_id, target_id)` columns. Backlinks are derived via reverse query (not materialized). The `[[link syntax]]` is parsed by a custom TipTap extension that stores links as inline nodes in the editor's ProseMirror document.

**Rationale**:

### Table design

```sql
CREATE TABLE IF NOT EXISTS document_links (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    target_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    UNIQUE(source_id, target_id)
);

CREATE INDEX IF NOT EXISTS idx_document_links_target_id ON document_links(target_id);
CREATE INDEX IF NOT EXISTS idx_document_links_source_id ON document_links(source_id);
```

The `UNIQUE(source_id, target_id)` constraint ensures only one link from A to B exists. Self-references (A to A) are allowed per spec. `ON DELETE CASCADE` ensures that when a document is permanently deleted, its links are cleaned up.

### Backlinks via reverse query (not materialized)

Forward links from document A: `SELECT target_id FROM document_links WHERE source_id = ?`
Backlinks to document B: `SELECT source_id FROM document_links WHERE target_id = ?`

Both queries hit indexed columns and execute in under 1 ms even at 10,000 links. There is no need for a separate `backlinks` table or materialized view. The reverse query approach is simpler and avoids the consistency problem of keeping two tables in sync.

### Link syntax parsing

When the user types `[[` in TipTap, a custom ProseMirror `InputRule` or `Suggestion` plugin activates autocompletion (see Decision 9). Once a document is selected, the editor inserts a custom inline node of type `docLink` with attributes `{ docId, title }`. When the document is saved, the backend:

1. Receives the HTML body containing `<span data-doc-link="42">Some Title</span>` elements.
2. Parses all `data-doc-link` attributes to extract target document IDs.
3. Diffs the current set of links against the stored set in `document_links`.
4. Inserts new links, deletes removed links.

This approach keeps the link table as a derived index of the document content, ensuring it never drifts from the actual body.

**Alternatives considered**:

- **Materialized backlinks table**: A separate `document_backlinks` table updated via triggers. Adds complexity (two tables, trigger maintenance) for no measurable performance gain at this scale.
- **Parse `[[wikilink]]` syntax as plain text**: Store `[[Document Title]]` as literal text and parse it on read. Fragile -- renaming a document would break all links. The inline node approach stores the document ID, making links rename-safe.
- **Store links only in the link table (not in body)**: Would require the editor to look up link metadata on load. Storing the link as an inline node in the body means the editor is self-contained and the link table is a derived cache.

---

## Decision 5: External content capture

**Decision**: Implement capture via three mechanisms: (1) a PWA `share_target` in `manifest.webmanifest`, (2) a `POST /api/capture` endpoint, and (3) Go-side Open Graph metadata extraction using `net/html` parsing.

**Rationale**:

### PWA share_target

The existing `manifest.webmanifest` already defines the PWA. Adding `share_target` enables mobile "Share to PKD":

```json
{
  "share_target": {
    "action": "/api/capture",
    "method": "POST",
    "enctype": "application/x-www-form-urlencoded",
    "params": {
      "title": "title",
      "text": "text",
      "url": "url"
    }
  }
}
```

When the user shares from another app, the OS sends a POST to `/api/capture` with form-encoded `title`, `text`, and/or `url` fields. The service worker intercepts this in a `fetch` event handler and forwards it to the API (adding the auth cookie).

### API endpoint

```
POST /api/capture
Content-Type: application/json

{
  "title": "optional title",
  "content": "optional body text or HTML",
  "url": "https://example.com/article",
  "tags": ["captura"]
}

Response: 201 Created
{
  "id": 123,
  "title": "Article Title (from OG or provided)",
  ...
}
```

The endpoint is authenticated (same session cookie as all other API routes). It creates a new document with tag `#captura` by default. If a `url` is provided, the backend fetches and parses Open Graph metadata.

### Open Graph extraction (Go-side)

Use `golang.org/x/net/html` (already an indirect dependency via bluemonday) to parse the HTML `<head>` of the target URL and extract `og:title`, `og:description`, and `og:image` from `<meta property="og:*">` tags. This is a best-effort operation -- if the fetch fails or the page has no OG tags, the capture proceeds with whatever the user provided.

No external Go library is needed. The extraction is ~50 lines of code: HTTP GET with a 5-second timeout, parse the tokenized HTML, scan for `<meta>` tags with `property` attribute starting with `og:`.

**Alternatives considered**:

- **Third-party OG library (e.g., `github.com/dyatlov/go-opengraph`)**: Adds a dependency for something achievable with stdlib + `x/net/html`. YAGNI.
- **Client-side OG extraction via CORS proxy**: Would require a proxy endpoint and faces CORS issues. Server-side is simpler.
- **Offline queue with IndexedDB**: Rejected in Q3. Adds significant complexity (IndexedDB store, sync logic, conflict resolution) for a feature that is unlikely to be used offline.

---

## Decision 6: Svelte build pipeline in Docker

**Decision**: Use a three-stage Dockerfile: (1) Node.js stage to build the Svelte frontend, (2) Go stage to build the backend binary (embedding the built frontend), (3) distroless runtime stage.

**Rationale**: The current Dockerfile is two-stage (Go build + distroless). The frontend is embedded via `go:embed` from `internal/server/web/`. The refactor adds a Node.js build step before the Go build.

```dockerfile
# Stage 1: Frontend build
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build
# Output: /app/frontend/dist/

# Stage 2: Go build
FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/frontend/dist/ ./internal/server/web/dist/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/pkd ./cmd/pkd

# Stage 3: Runtime
FROM gcr.io/distroless/static-debian12
COPY --from=backend /out/pkd /pkd
EXPOSE 8080
ENTRYPOINT ["/pkd"]
```

### Image size budget (target: 30 MB)

| Component | Estimated size |
|-----------|---------------|
| distroless/static base | ~2 MB |
| Go binary (stripped, no CGO) | ~18 MB |
| Embedded frontend (Svelte build + TipTap + D3) | ~1.5 MB uncompressed |
| **Total** | **~21.5 MB** |

The current image without the Svelte frontend is already ~20 MB. Adding the compiled Svelte bundle (typically 200-400 KB gzipped, ~1-1.5 MB uncompressed) stays well within the 30 MB target. The Node.js and Go build stages are discarded in the final image.

Key Vite build settings for size:
- `build.minify: 'esbuild'` (default, fast).
- `build.rollupOptions.output.manualChunks`: Split TipTap and D3 into separate chunks for better caching.
- `build.assetsInlineLimit: 4096`: Inline small assets as base64.

**Alternatives considered**:

- **Pre-build frontend in CI, commit dist/**: Avoids the Node.js Docker stage but pollutes the repo with build artifacts and creates merge conflicts. The three-stage build is cleaner.
- **esbuild instead of Vite**: Faster builds but less ecosystem support for Svelte. Vite already uses esbuild for transforms internally.
- **Bun instead of Node.js**: Faster install/build, but less mature in Alpine Docker images. Not worth the risk for a CI build step that runs in <30 seconds.

---

## Decision 7: C4 Model documentation

**Decision**: Place C4 Model documentation in `docs/c4/` using Mermaid diagrams in Markdown files. Four files, one per C4 level.

**Rationale**: The spec requires C4 Model documentation at four levels (FR-100 through FR-102). Mermaid is mandated for GitHub-native rendering.

Directory structure:

```
docs/c4/
  context.md    -- Level 1: System Context
  container.md  -- Level 2: Containers (Go backend, Svelte frontend, SQLite, volumes)
  component.md  -- Level 3: Components (chi router, stores, sessions, handlers)
  code.md       -- Level 4: Code-level (key structs, interfaces)
```

Each file contains:
1. A brief textual description of what the diagram shows.
2. A Mermaid diagram (using `graph TD` or `C4Context`/`C4Container` from Mermaid's C4 extension).
3. A legend/notes section explaining key relationships.

Mermaid supports C4 diagram types natively since v10.0 (`C4Context`, `C4Container`, `C4Component`, `C4Dynamic`). These render as proper C4 boxes with stereotypes on GitHub.

**Alternatives considered**:

- **Structurizr DSL + exported images**: More "proper" C4 tooling but requires a separate tool to render. Mermaid renders natively on GitHub with zero tooling.
- **PlantUML**: Requires a server or CLI to render. Not natively supported by GitHub.
- **Single file with all 4 levels**: Too long. Separate files allow linking directly to a specific level.

---

## Decision 8: Frontend routing

**Decision**: Use hash-based client-side routing (`#/path`) without a routing library.

**Rationale**: The current app is a single-page application served from `index.html`. The Go backend serves `index.html` for the root path `/` and API routes under `/api/`. Hash-based routing (`/#/doc/42`, `/#/graph`, `/#/calendar`) works without any backend changes -- the server always serves `index.html` for `/` and the fragment is handled entirely client-side.

The app has a small number of routes:

- `#/` -- document tree (default)
- `#/doc/{id}` -- edit document
- `#/graph` -- graph view
- `#/calendar` -- calendar view
- `#/admin` -- admin panel
- `#/search?q=...` -- search results

With ~6 routes, a full routing library is unnecessary. A simple reactive router can be implemented in ~30 lines of Svelte:

```svelte
<script>
  let hash = $state(location.hash.slice(1) || '/');
  window.addEventListener('hashchange', () => hash = location.hash.slice(1) || '/');
</script>

{#if hash.startsWith('/doc/')}
  <DocumentEditor id={hash.split('/')[2]} />
{:else if hash === '/graph'}
  <GraphView />
{:else if hash === '/calendar'}
  <CalendarView />
{:else if hash === '/admin'}
  <AdminPanel />
{:else}
  <DocumentTree />
{/if}
```

**Alternatives considered**:

| Option | Trade-off |
|--------|-----------|
| `svelte-spa-router` | Hash-based routing library. Adds ~4 KB for features (named params, guards) not needed in a 6-route app. |
| `@sveltejs/kit` (SvelteKit) | Full framework with file-based routing, SSR, etc. Massive overkill for a single-user PWA with a Go backend. Would require rethinking the entire backend integration. |
| History API (`/doc/42`) | Requires the Go backend to serve `index.html` for all unmatched paths (catch-all route). Works but adds backend complexity and breaks if the user bookmarks a path and the server does not catch it. Hash routing works out of the box. |

---

## Decision 9: Link autocompletion in TipTap

**Decision**: Build a custom TipTap extension using TipTap's `Suggestion` utility (from `@tiptap/suggestion`) that triggers on `[[`, fetches the document list from the backend, and inserts an inline `docLink` node.

**Rationale**: TipTap's `Suggestion` utility is designed for exactly this use case -- it handles trigger character detection, popup positioning, keyboard navigation, and selection. It is the same utility used by TipTap's built-in `@mention` extension.

### Extension design

1. **Trigger**: The user types `[[`. The Suggestion plugin detects this and opens a floating autocomplete popup.
2. **Query**: As the user continues typing, the Suggestion plugin calls a `items` function with the current query string. This function calls `GET /api/documents/search?q={query}&limit=10` (a lightweight endpoint returning `[{id, title, icon}]`).
3. **Selection**: The user picks a document from the list (click or Enter). The Suggestion plugin calls `command`, which inserts a `docLink` inline node: `<span data-doc-link="42" class="doc-link">Document Title</span>`.
4. **Rendering**: The `docLink` node renders as a styled inline element with an icon and the document title. Clicking it navigates to `#/doc/42`.

### Debouncing

The search query is debounced at 150 ms to avoid excessive API calls while typing.

### Preloading

For small collections (<500 documents), the full document list can be fetched once on editor mount and filtered client-side. For larger collections, the API search is used. The threshold can be determined at load time via the tree endpoint which already returns all documents.

**Alternatives considered**:

- **Custom ProseMirror plugin from scratch**: TipTap's `Suggestion` utility already abstracts the hard parts (caret position tracking, popup lifecycle, keyboard nav). Writing this from scratch would be ~300 lines of ProseMirror code vs ~50 lines using Suggestion.
- **Markdown-style parsing on save**: Parse `[[Document Title]]` as plain text on save and resolve to IDs. This breaks if two documents have the same title and provides no autocompletion UX. The inline node approach stores the ID at insertion time.
- **Link dialog (button in toolbar)**: Less fluid than inline `[[` syntax. The toolbar button can be added later as a secondary insertion method, but the `[[` trigger is the primary UX for knowledge workers (familiar from Obsidian, Notion, Logseq).

---

## Decision 10: Go backend changes

**Decision**: Extend the existing Go backend with new endpoints for links, graph data, and capture. Add the `document_links` table via the existing idempotent schema migration. The Go backend continues to embed and serve the frontend (now built by Vite instead of served as static files).

**Rationale**: The backend architecture (chi router, store pattern, middleware stack) is solid and does not need restructuring. The changes are additive.

### New endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/documents/{id}/links` | List forward links and backlinks for a document |
| GET | `/api/graph` | Return all linked documents as `{nodes: [...], links: [...]}` for D3 |
| POST | `/api/capture` | Create a document from external content (share target / API) |
| GET | `/api/documents/suggest?q=...` | Lightweight search for link autocompletion (returns `[{id, title, icon}]`) |

### Schema migration

The `document_links` table (see Decision 4) is added to `schema.sql`. Since all DDL uses `CREATE TABLE IF NOT EXISTS`, it is safe to add to the existing schema file -- existing databases will gain the new table on next startup without affecting existing tables.

### Link sync on document save

The existing `PUT /api/documents/{id}` handler is extended:

1. After saving the document body, parse `data-doc-link` attributes from the HTML.
2. Query the current links from `document_links` where `source_id = ?`.
3. Compute the diff (new links to insert, stale links to delete).
4. Execute inserts and deletes within the same transaction as the document save.

This ensures link consistency -- if the document save fails, no links are modified.

### Graph endpoint

`GET /api/graph` returns:

```json
{
  "nodes": [
    {"id": 1, "title": "Doc A", "icon": "book", "tags": ["project"]},
    ...
  ],
  "links": [
    {"source": 1, "target": 2},
    ...
  ]
}
```

By default, only documents with at least one link are included (per Q8). A `?all=true` query parameter includes isolated documents. The query is a JOIN between `documents` and `document_links` with tag information from the `document_tags` join table.

### Open Graph extraction

A new internal package `internal/opengraph` (or a function in `internal/server/handlers_capture.go`) handles URL fetching and meta tag parsing. It uses `net/http` for the GET request (5-second timeout, follow redirects, cap response body at 1 MB) and `golang.org/x/net/html` for tokenized parsing.

### Frontend embedding change

The current `go:embed` directive in `internal/server/assets.go` points to `internal/server/web/`. After the refactor, it points to `internal/server/web/dist/` (the Vite build output). The Go embedding mechanism is unchanged -- only the source directory changes.

**Alternatives considered**:

- **Separate links CRUD (POST/DELETE per link)**: Explicit link management endpoints. Rejected because links are derived from the document body -- managing them separately would create consistency issues. The save-and-sync approach treats the body as the source of truth.
- **GraphQL for graph data**: Overkill for a single endpoint. REST is already established in the codebase.
- **Rewrite backend in a different language**: The Go backend is working, tested, and well-structured. A rewrite would provide no benefit and delay the project.
