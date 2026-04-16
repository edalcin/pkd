# C4 Level 4 — Code: PKD

> **Versão**: v2 (003-pkm-refactor) · **Data**: 2026-04-16

## Modelo de dados — Structs Go (`internal/model/`)

```mermaid
classDiagram
    class Document {
        +int64 ID
        +int64* ParentID
        +string Title
        +string BodyHTML
        +string BodyText
        +string Icon
        +int Position
        +int64 Version
        +time.Time* TrashedAt
        +int64* OriginalParentID
        +[]string Tags
        +[]int64 AttachmentIDs
        +time.Time CreatedAt
        +time.Time UpdatedAt
    }

    class DocumentTreeNode {
        +int64 ID
        +int64* ParentID
        +string Title
        +string Icon
        +int Position
        +int64 Version
        +[]string Tags
        +[]*DocumentTreeNode Children
    }

    class Link {
        +int64 ID
        +int64 SourceID
        +int64 TargetID
        +time.Time CreatedAt
    }

    class LinkEntry {
        +int64 ID
        +int64 SourceID
        +int64 TargetID
        +string SourceTitle
        +string TargetTitle
        +bool TargetTrashed
        +time.Time CreatedAt
    }

    class LinksResponse {
        +[]LinkEntry Outgoing
        +[]LinkEntry Incoming
    }

    class GraphData {
        +[]GraphNode Nodes
        +[]GraphEdge Edges
    }

    class GraphNode {
        +int64 ID
        +string Title
        +string Icon
        +[]string Tags
    }

    class GraphEdge {
        +int64 Source
        +int64 Target
    }

    class VersionConflict {
        +int64 StoredVersion
        +*Document Stored
    }

    Document "1" --> "*" Document : parent_id
    Document "1" --> "*" Link : como source_id
    Document "1" --> "*" Link : como target_id
    LinksResponse --> LinkEntry
    GraphData --> GraphNode
    GraphData --> GraphEdge
    Document --> VersionConflict : retornado em 409
```

## Fluxo de sincronização de links (atomicidade)

O link sync ocorre dentro da mesma transação SQL do save do documento, garantindo que o corpo HTML e a tabela `document_links` estejam sempre consistentes.

```mermaid
sequenceDiagram
    participant Browser
    participant Handler as handlers_documents.go
    participant DocStore as store/documents.go
    participant LinkStore as store/links.go
    participant SQLite

    Browser->>Handler: PUT /api/documents/{id}<br/>{version, title, body_html, body_text, icon}
    Handler->>Handler: bluemonday.SanitizeHTML(body_html)
    Handler->>DocStore: UpdateAndSync(id, version, ..., syncFn)
    DocStore->>SQLite: BEGIN TRANSACTION
    DocStore->>SQLite: SELECT version FROM documents WHERE id=?
    alt versão desatualizada
        DocStore-->>Handler: ErrVersionConflict
        Handler-->>Browser: 409 {stored_version, stored}
    else versão ok
        DocStore->>SQLite: UPDATE documents SET title=?, body_html=?, ...
        DocStore->>LinkStore: syncFn(tx) = SyncLinksFromHTML(tx, docID, html)
        LinkStore->>LinkStore: extractDocLinkIDs(html)<br/>tokeniza data-doc-link attrs
        LinkStore->>SQLite: SELECT id, target_id FROM document_links WHERE source_id=?
        LinkStore->>SQLite: INSERT OR IGNORE (novos links)
        LinkStore->>SQLite: DELETE (links removidos)
        DocStore->>SQLite: COMMIT
        DocStore-->>Handler: *Document (salvo)
        Handler-->>Browser: 200 Document JSON
    end
```

## Fluxo de criação de link bidirecional no editor

```mermaid
sequenceDiagram
    participant User
    participant Editor as Editor.svelte
    participant Suggestion as link-suggestion.js
    participant TipTap
    participant API as /api/search

    User->>Editor: digita [[
    Editor->>Suggestion: Suggestion plugin detecta trigger [[
    Suggestion->>API: GET /api/search?q={query} (debounce 150ms)
    API-->>Suggestion: [{id, title, snippet}, ...]
    Suggestion->>Editor: renderiza dropdown de documentos
    User->>Editor: seleciona "Documento B"
    Editor->>TipTap: insertContent({type: 'docLink', attrs: {docId: 42, docTitle: "Documento B"}})
    TipTap->>Editor: renderiza <span data-doc-link="42" class="doc-link">Documento B</span>
    Note over Editor: auto-save dispara em 2s
    Editor->>API: PUT /api/documents/{id} com body_html contendo data-doc-link="42"
    Note over API: SyncLinksFromHTML extrai ID 42,<br/>insere em document_links se não existir
    Editor->>API: GET /api/documents/{docId}/links
    API-->>Editor: {outgoing: [...], incoming: [{source_id: 42, source_title: "Documento B", ...}]}
    Editor->>User: painel "Referenciado por" atualizado
```

## Extração de Open Graph (captura de conteúdo)

```mermaid
flowchart TD
    A[POST /api/capture\njson ou form-encoded] --> B{Tem 'url'?}
    B -- não --> E[Criar documento\ncom conteúdo fornecido]
    B -- sim --> C[HTTP GET url\ntimeout 5s, max 1MB]
    C --> D{Fetch ok?}
    D -- falha silenciosa --> E
    D -- ok --> F[html.NewTokenizer\nscandeia meta property=og:title\nmeta property=og:description]
    F --> G{og:title encontrado?}
    G -- sim e title vazio --> H[usa og:title como título]
    G -- não --> I[usa title fornecido ou timestamp]
    H --> E
    I --> E
    E --> J[documents.Create + Update]
    J --> K[tags.SetDocumentTags captura + extras]
    K --> L[201 Document JSON]
```

## Schema SQL — tabela document_links (nova em v2)

```sql
CREATE TABLE IF NOT EXISTS document_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    target_id   INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at  TEXT    NOT NULL,
    UNIQUE(source_id, target_id)   -- único link de A→B; A→B e B→A são registros independentes
);

CREATE INDEX IF NOT EXISTS idx_document_links_source ON document_links(source_id);
CREATE INDEX IF NOT EXISTS idx_document_links_target ON document_links(target_id);
-- idx_document_links_target viabiliza a query de backlinks:
-- SELECT * FROM document_links WHERE target_id = ? (quem referencia documento X)
```

**Decisões de design:**
- **Sem rótulo ou tipo** — modelo Obsidian/Logseq: simplicidade máxima
- **Backlinks derivados** — `WHERE target_id = X` com índice; sem tabela materializada
- **CASCADE on DELETE** — links removidos automaticamente quando documento é permanentemente excluído
- **Links mantidos no trash** — quando documento vai para lixeira, links permanecem; UI marca como "quebrado"
- **Autorreferência permitida** — A pode linkar para A (uso válido: seções do mesmo documento)

## Roteamento hash-based (frontend)

```mermaid
flowchart LR
    URL["window.location.hash"] --> Router["App.svelte\ngetRoute()"]
    Router -->|"#/"| Home["Empty state\n(nenhum doc selecionado)"]
    Router -->|"#/doc/{id}"| EditorComp["Editor.svelte\ndocId={id}"]
    Router -->|"#/graph"| GraphComp["GraphView.svelte"]
    Router -->|"#/calendar"| CalComp["Calendar.svelte"]
    Router -->|"#/admin"| AdminComp["Admin.svelte"]
    Router -->|"#/search?q=..."| SearchPage["Search results"]
```

O servidor Go serve apenas `/` → `index.html`. O fragmento `#...` nunca chega ao servidor — é tratado exclusivamente no cliente. Isso elimina a necessidade de catch-all no backend e permite PWA offline sem service worker complexo.

## Endpoints REST — adições v2

| Método | Caminho | Handler | Descrição |
|---|---|---|---|
| GET | `/api/documents/{id}/links` | `handleListLinks` | `{outgoing: [LinkEntry], incoming: [LinkEntry]}` |
| POST | `/api/documents/{id}/links` | `handleCreateLink` | Body: `{target_id}` → 201 LinkEntry |
| DELETE | `/api/documents/{id}/links/{linkId}` | `handleDeleteLink` | 204 sem corpo |
| GET | `/api/graph` | `handleGraph` | `{nodes: [GraphNode], edges: [GraphEdge]}`. Query: `?tag=&all=true` |
| POST | `/api/capture` | `handleCapture` | JSON ou form-encoded. Cria doc + tag #captura + Open Graph |
