# C4 Level 4 — Code: PKD

> **Versão**: v2.3 · **Data**: 2026-05-29

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

    class Tag {
        +int64 ID
        +string Name
        +string Color
    }

    class TagWithCount {
        +int64 ID
        +string Name
        +string Color
        +int Count
    }

    class Attachment {
        +int64 ID
        +int64 DocumentID
        +string OriginalName
        +string StoredFilename
        +string MimeType
        +int64 SizeBytes
        +string URL
        +time.Time CreatedAt
    }

    class AttachmentWithDoc {
        +Attachment
        +string DocumentTitle
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
    Document "1" --> "*" Attachment : document_id
    AttachmentWithDoc --> Attachment : embeds
    TagWithCount --> Tag : extends
    LinksResponse --> LinkEntry
    GraphData --> GraphNode
    GraphData --> GraphEdge
    class DocumentVersion {
        +int64 ID
        +int64 DocumentID
        +string Title
        +string BodyHTML
        +string Icon
        +string ContentSHA256
        +time.Time CreatedAt
    }

    Document --> VersionConflict : retornado em 409
    Document "1" --> "*" DocumentVersion : document_id (CASCADE)
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

## Fluxo de carregamento de sub-documentos (cards)

Quando um documento é aberto no editor, filhos diretos são carregados para exibição como cards.

```mermaid
sequenceDiagram
    participant Editor as Editor.svelte
    participant API as Go HTTP Server
    participant SQLite

    Editor->>API: GET /api/documents/{id}/children
    API->>SQLite: SELECT ... FROM documents WHERE parent_id=? AND trashed_at IS NULL ORDER BY position
    SQLite-->>API: rows
    API-->>Editor: [Document, ...] (array vazio [] se sem filhos)
    alt children.length > 0
        Editor->>Editor: renderiza .children-grid com cards clicáveis
    else
        Editor->>Editor: seção oculta (nada renderizado)
    end
```

## Fluxo de exclusão permanente com integridade referencial

```mermaid
flowchart TD
    A[Admin: Excluir da lixeira] --> B[handleAdminDeleteTrashItem]
    B --> C[attachments.DeleteByDocument\nremove arquivos do disco + rows do DB]
    C --> D[docs.PermanentDelete]
    D --> E[BEGIN TX]
    E --> F["UPDATE documents SET parent_id=NULL\nWHERE parent_id=? (detach children)"]
    F --> G["DELETE FROM documents\nWHERE id=? AND trashed_at IS NOT NULL"]
    G --> H[COMMIT]
    H --> I[tags.PruneUnused\nDELETE tags não referenciadas]
    I --> J[200 OK]
```

> **Por que o detach explícito?** SQLite com `PRAGMA foreign_keys=ON` e `ON DELETE RESTRICT` bloqueia o `DELETE` se existirem filhos. Documentos filhos não-lixados devem ser promovidos para root, não excluídos junto com o pai.

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

## Schema SQL — tabela tags (coluna color adicionada)

```sql
CREATE TABLE IF NOT EXISTS tags (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    name  TEXT    NOT NULL UNIQUE,
    color TEXT    NOT NULL DEFAULT ''   -- hex color, e.g. '#ef4444'; vazio = cor padrão
);
```

A coluna `color` é adicionada via migração idempotente:

```go
{`ALTER TABLE tags ADD COLUMN color TEXT NOT NULL DEFAULT ''`, "alter tags color"},
```

## Schema SQL — tabela document_links

```sql
CREATE TABLE IF NOT EXISTS document_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    target_id   INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at  TEXT    NOT NULL,
    UNIQUE(source_id, target_id)
);

CREATE INDEX IF NOT EXISTS idx_document_links_source ON document_links(source_id);
CREATE INDEX IF NOT EXISTS idx_document_links_target ON document_links(target_id);
```

**Decisões de design:**
- **Sem rótulo ou tipo** — modelo Obsidian/Logseq: simplicidade máxima
- **Backlinks derivados** — `WHERE target_id = X` com índice; sem tabela materializada
- **CASCADE on DELETE** — links removidos automaticamente quando documento é permanentemente excluído
- **Links mantidos no trash** — quando documento vai para lixeira, links permanecem; UI marca como "quebrado"

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

O servidor Go serve apenas `/` → `index.html` (com `Cache-Control: no-cache` para garantir que o browser sempre busque o shell mais recente após deploys). O fragmento `#...` nunca chega ao servidor — tratado exclusivamente no cliente.

## Endpoints REST

### Documentos

| Método | Caminho | Handler | Descrição |
|---|---|---|---|
| POST | `/api/documents` | `handleCreateDocument` | Cria documento. Body: `{parent_id?, title}` |
| GET | `/api/documents/{id}` | `handleGetDocument` | Retorna documento com tags e attachment_ids |
| PUT | `/api/documents/{id}` | `handleUpdateDocument` | Salva com versionamento otimista; sincroniza links |
| DELETE | `/api/documents/{id}` | `handleDeleteDocument` | Soft-delete (move para lixeira) |
| POST | `/api/documents/{id}/move` | `handleMoveDocument` | Body: `{new_parent_id}` |
| POST | `/api/documents/{id}/restore` | `handleRestoreDocument` | Restaura da lixeira |
| GET | `/api/documents/{id}/children` | `handleListChildren` | Filhos diretos não-lixados para cards de sub-docs |

### Links bidirecionais

| Método | Caminho | Handler | Descrição |
|---|---|---|---|
| GET | `/api/documents/{id}/links` | `handleListLinks` | `{outgoing: [LinkEntry], incoming: [LinkEntry]}` |
| POST | `/api/documents/{id}/links` | `handleCreateLink` | Body: `{target_id}` → 201 LinkEntry |
| DELETE | `/api/documents/{id}/links/{linkId}` | `handleDeleteLink` | 204 sem corpo |

### Administração

| Método | Caminho | Handler | Descrição |
|---|---|---|---|
| GET | `/api/admin/trash` | `handleAdminListTrash` | Lista todos os documentos na lixeira |
| DELETE | `/api/admin/trash/{id}` | `handleAdminDeleteTrashItem` | Exclui permanentemente (com limpeza de anexos e tags) |
| POST | `/api/admin/trash/empty` | `handleAdminEmptyTrash` | Esvazia lixeira completa |
| GET | `/api/admin/tags` | `handleAdminListAllTags` | Lista todas as tags (LEFT JOIN, inclui count=0) |
| PUT | `/api/admin/tags/{id}` | `handleAdminUpdateTag` | Atualiza nome e/ou cor da tag |
| DELETE | `/api/admin/tags/{id}` | `handleAdminDeleteTag` | Remove tag e suas associações |
| POST | `/api/admin/tags/prune` | `handleAdminPruneTags` | Remove tags sem nenhum documento |
| GET | `/api/admin/attachments` | `handleAdminListAttachments` | Lista todos os anexos com título do documento |
| GET | `/api/admin/attachments/orphans` | `handleAdminListOrphans` | Lista órfãos como `[]OrphanInfo{key, size_bytes}` |
| GET | `/api/admin/attachments/orphans/download` | `handleAdminDownloadOrphan` | Download de arquivo órfão (`?key=`); verifica se é órfão antes de servir |
| DELETE | `/api/admin/attachments/orphans/item` | `handleAdminDeleteOrphan` | Exclui arquivo órfão (`?key=`); verifica se é órfão antes de deletar |
| GET | `/api/documents/{id}/versions` | `handleListVersions` | Lista snapshots sem body_html |
| GET | `/api/documents/{id}/versions/{vid}` | `handleGetVersion` | Snapshot completo com body_html |
| POST | `/api/documents/{id}/versions/{vid}/restore` | `handleRestoreVersion` | Restaura snapshot; retorna Document atualizado |
| DELETE | `/api/documents/{id}/versions/{vid}` | `handleDeleteVersion` | Exclui snapshot individual |
| GET | `/api/admin/settings` | `handleAdminGetSettings` | Lê configurações persistidas (whitelist) |
| PUT | `/api/admin/settings` | `handleAdminPutSettings` | Salva configuração (`versions.max_per_doc`) |
| POST | `/api/admin/backup` | `handleAdminBackup` | Download do SQLite via VACUUM INTO |
| POST | `/api/admin/restore` | `handleAdminRestore` | Restaura backup (requer confirmação) |
| POST | `/api/admin/cleanup` | `handleAdminCleanup` | VACUUM no banco de dados |
| POST | `/api/admin/check-urls` | `handleAdminCheckURLs` | Testa validade de links externos (HTTP HEAD) |

### Outros

| Método | Caminho | Descrição |
|---|---|---|
| GET | `/api/graph` | `{nodes, edges}` para D3.js. Query: `?tag=&all=true` |
| POST | `/api/capture` | Cria doc a partir de URL/texto; extrai Open Graph |
| GET | `/api/tags` | Tags ativas (INNER JOIN — exclui docs na lixeira) |
| GET | `/api/search` | Busca FTS5. Query: `?q=` |
| GET | `/api/documents/{id}/attachments` | Lista anexos do documento |
| POST | `/api/documents/{id}/attachments` | Upload (multipart ou octet-stream) |
| GET | `/api/attachments/{id}` | Serve arquivo com Content-Disposition correto |
| DELETE | `/api/attachments/{id}` | Remove anexo |
