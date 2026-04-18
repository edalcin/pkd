# C4 Level 3 — Component: PKD

> **Versão**: v2.1 · **Data**: 2026-04-18

## Descrição

O backend Go está organizado em pacotes independentes sob `internal/`. O roteador chi conecta todos os componentes. O frontend Svelte é estruturado em stores reativos e componentes.

## Diagrama de Componentes — Backend Go

```mermaid
C4Component
    title Component Diagram — PKD Go Backend

    Container_Boundary(server_pkg, "internal/server") {
        Component(router, "chi Router + Middleware Stack", "go-chi/chi v5", "Monta todas as rotas. Middleware global: RequestID, RealIP, Recoverer, SecurityHeaders, CSRF.")
        Component(auth_mw, "AuthRequired Middleware", "middleware_auth.go", "Exige cookie de sessão válido para rotas protegidas. Retorna 401 sem redirecionar.")
        Component(throttle_mw, "Throttle Middleware", "middleware_throttle.go", "Rate limiting por IP: 5 falhas → bloqueio de 30 min. Responde com Retry-After.")
        Component(security_mw, "SecurityHeaders + CSRF", "middleware_security.go, middleware_csrf.go", "CSP com object-src e frame-src 'self' para embed de PDFs/imagens. HSTS, X-Frame-Options, X-Content-Type-Options. CSRF double-submit cookie.")
        Component(doc_handlers, "Document Handlers", "handlers_documents.go", "CRUD de documentos com versionamento, soft-delete, restore, move hierárquico. handleListChildren retorna filhos diretos para cards de sub-documentos. Chama UpdateAndSync para sincronizar links na mesma transação.")
        Component(link_handlers, "Link Handlers", "handlers_links.go", "GET/POST/DELETE /api/documents/{id}/links. Lista links de saída e backlinks com títulos e status trashed.")
        Component(att_handlers, "Attachment Handlers", "handlers_attachments.go", "Upload, download e delete de anexos. Content-Disposition: inline para PDFs, imagens, áudio, vídeo. Override de X-Frame-Options e CSP para permitir embed no browser.")
        Component(admin_handlers, "Admin Handlers", "handlers_admin.go", "Lixeira (list/delete/empty), backup/restore, limpeza. Tags: list-all, update (nome+cor), delete, prune-orphans. Attachments: list-all (com doc title), list-orphans.")
        Component(graph_handler, "Graph Handler", "handlers_graph.go", "GET /api/graph. Retorna nodes + edges para D3.js. Suporta filtro por tag e toggle all-docs.")
        Component(capture_handler, "Capture Handler", "handlers_capture.go", "POST /api/capture. Aceita JSON e form-encoded (PWA share_target). Extrai Open Graph de URLs.")
        Component(other_handlers, "Other Handlers", "handlers_*.go (8 arquivos)", "Tags, busca FTS5, calendário, share links, auth, health, PWA.")
        Component(assets, "Static Assets", "assets.go + web/dist/", "Serve SPA Svelte embutida via //go:embed. index.html com Cache-Control: no-cache para forçar reload após deploys.")
    }

    Container_Boundary(store_pkg, "internal/store") {
        Component(doc_store, "DocumentStore", "documents.go", "Create, GetByID, Update, UpdateAndSync (aceita syncFn(*sql.Tx)), SoftDelete, Restore, Move, ListTree, ListChildren (filhos diretos para cards), PermanentDelete (remove filhos órfãos antes de deletar), EmptyTrash (detach children + delete trashed).")
        Component(link_store, "LinkStore", "links.go", "CreateLink, DeleteLink, GetLinksForDocument, GetGraphData, SyncLinksFromHTML (tokeniza HTML, diff, insert/delete no mesmo tx).")
        Component(tag_store, "TagStore", "tags.go", "ListWithCounts (INNER JOIN, exclui tags de docs na lixeira), ListAllWithCounts (LEFT JOIN, inclui todas — para admin), SetDocumentTags, RenameOrMerge, Update (nome+cor), Delete, PruneUnused.")
        Component(att_store, "AttachmentStore", "attachments.go", "CreateFile, GetByID, ListByDocument, ListAllWithDocument (join com título do doc), Delete, DeleteByDocument (limpa disco+DB para um doc), ListOrphans, FullPath.")
        Component(search_store, "SearchStore", "search.go", "FTS5 full-text search com snippets. Índice contentless mantido pela app.")
        Component(share_store, "ShareStore", "shares.go", "Geração de token SHA-256, lookup constant-time, revogação por revoked_at.")
        Component(backup_store, "BackupStore", "backup.go", "VACUUM INTO para backup, restauração atômica.")
        Component(migrate, "Schema Migration", "migrate.go + schema.sql", "DDL idempotente a cada inicialização. Inclui migração de coluna color na tabela tags.")
    }

    Container_Boundary(security_pkg, "internal/security") {
        Component(sanitize, "HTML Sanitizer", "sanitize.go", "bluemonday AllowList: formatos, imagens, tabelas, links http/https. Remove scripts, event handlers, javascript: URIs.")
        Component(icons, "Icon Validator", "icons.go", "Allowlist de ícones válidos (emojis). Rejeita valores inválidos na API.")
        Component(tokens, "Token Generator", "tokens.go", "crypto/rand para sessões e share tokens. ConstantTimeCompare para password e share lookups.")
        Component(paths, "Path Validator", "paths.go", "Previne path traversal: rejeita .., null bytes, caminhos absolutos; verifica que resultado final está sob base.")
    }

    Container_Boundary(sessions_pkg, "internal/sessions") {
        Component(sess_store, "Session Store", "store.go", "sync.Map em memória: sessionID → {IP, CreatedAt, LastSeenAt}. Não persistido. Expira por idle timeout.")
    }

    Rel(router, auth_mw, "Aplica a rotas autenticadas")
    Rel(router, throttle_mw, "Aplica a /api/login")
    Rel(router, security_mw, "Aplica globalmente")
    Rel(router, doc_handlers, "Roteia /api/documents/*")
    Rel(router, link_handlers, "Roteia /api/documents/{id}/links/*")
    Rel(router, att_handlers, "Roteia /api/attachments/* e /api/documents/{id}/attachments")
    Rel(router, admin_handlers, "Roteia /api/admin/*")
    Rel(router, graph_handler, "Roteia /api/graph")
    Rel(router, capture_handler, "Roteia /api/capture")
    Rel(router, other_handlers, "Roteia demais endpoints")
    Rel(doc_handlers, doc_store, "CRUD de documentos + ListChildren")
    Rel(doc_handlers, link_store, "SyncLinksFromHTML via UpdateAndSync")
    Rel(link_handlers, link_store, "CRUD de links")
    Rel(graph_handler, link_store, "GetGraphData")
    Rel(capture_handler, doc_store, "Create + Update")
    Rel(capture_handler, tag_store, "SetDocumentTags")
    Rel(admin_handlers, doc_store, "EmptyTrash, PermanentDelete")
    Rel(admin_handlers, att_store, "ListAllWithDocument, ListOrphans, DeleteByDocument")
    Rel(admin_handlers, tag_store, "ListAllWithCounts, Update, Delete, PruneUnused")
    Rel(doc_handlers, sanitize, "Sanitiza HTML antes de salvar")
    Rel(capture_handler, sanitize, "Sanitiza conteúdo capturado")
    Rel(router, sess_store, "Verifica sessão via AuthRequired")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## Diagrama de Componentes — Frontend Svelte

```mermaid
C4Component
    title Component Diagram — PKD Svelte 5 SPA

    Container_Boundary(svelte_app, "frontend/src/") {
        Component(app, "App.svelte", "Svelte 5, hash router", "Raiz da aplicação. Verifica sessão, tema, router hash-based (#/doc/:id, #/graph, #/calendar, #/admin). Monta sidebar + área de conteúdo.")

        Container_Boundary(stores, "lib/stores/") {
            Component(auth_store, "auth.js", "Svelte writable", "authenticated (null|bool), login(), logout(), checkSession(). Lê pkd_csrf cookie via api.js.")
            Component(doc_store_fe, "documents.js", "Svelte writable", "tree, activeDoc, loadTree(), saveDoc(), createDoc(), trashDoc(), moveDoc().")
            Component(tag_store_fe, "tags.js", "Svelte writable", "tags[] (com campo color), loadTags(), setDocumentTags(). Usado para chips coloridos em sidebar e editor.")
            Component(search_store_fe, "search.js", "Svelte writable", "debouncedSearch() 150ms, searchResults[], clearSearch().")
        }

        Container_Boundary(components_fe, "lib/components/") {
            Component(sidebar, "Sidebar.svelte", "Svelte 5", "Filtro de tags com chips coloridos, árvore de documentos delegada a TreeNode.svelte, botão novo documento.")
            Component(tree_node, "TreeNode.svelte", "Svelte 5 (recursivo)", "Nó da árvore: toggle expandir, drag-and-drop, ações contextuais.")
            Component(editor, "Editor.svelte", "TipTap v2, Svelte 5", "Editor rico com auto-save 2s, conflito de versão 409. Toolbar: formatação, tabela, imagem por URL, alinhamento, destaque com cor. Cards de sub-documentos (filhos diretos) exibidos entre o editor e a área de associações. Chips de tag com cor inline. Upload de anexos, modal de preview, backlinks, links externos.")
            Component(graph_view, "GraphView.svelte", "D3.js (d3-force, d3-zoom)", "Simulação force-directed em $effect. Svelte {#each} renderiza SVG circles/lines. Filtro de tags e toggle all-docs.")
            Component(search_comp, "Search.svelte", "Svelte 5", "Input de busca universal com dropdown. Debounce 150ms.")
            Component(calendar, "Calendar.svelte", "Svelte 5", "Grade mensal de dias. Clicar num dia lista documentos criados naquele dia.")
            Component(admin, "Admin.svelte", "Svelte 5", "Abas: Backup/Restore, Lixeira, Tags (color picker + inline edit + delete + prune), Arquivos (grid com thumbnail + orphans), Limpeza, Links.")
            Component(share_dialog, "ShareDialog.svelte", "Svelte 5", "Gera link público, copia URL, revoga link.")
        }

        Container_Boundary(editor_exts, "lib/editor/") {
            Component(doclink_ext, "doclink-extension.js", "TipTap Node", "Nó inline customizado. Atributo data-doc-link='{id}'. Click navega para #/doc/{id}.")
            Component(link_suggestion, "link-suggestion.js", "TipTap Suggestion", "Trigger: [[ — dropdown de autocomplete via /api/search. Debounce 150ms.")
        }

        Component(api_js, "lib/api.js", "Fetch wrapper", "apiFetch(), apiGet(), apiPost(), apiPut(), apiDelete(). Injeta X-CSRF-Token em requisições mutantes.")
    }

    Rel(app, auth_store, "Verifica sessão; exibe LoginPage se não autenticado")
    Rel(app, sidebar, "Monta na esquerda")
    Rel(app, editor, "Monta quando rota é #/doc/:id")
    Rel(app, graph_view, "Monta quando rota é #/graph")
    Rel(sidebar, tag_store_fe, "Chips coloridos via campo color")
    Rel(sidebar, doc_store_fe, "Carrega árvore, cria/move/deleta documentos")
    Rel(sidebar, tree_node, "Renderiza nós recursivamente")
    Rel(editor, doc_store_fe, "Carrega e salva documento ativo")
    Rel(editor, tag_store_fe, "Chips coloridos + autocomplete de tags")
    Rel(editor, api_js, "GET /api/documents/{id}/children para cards de sub-docs")
    Rel(editor, doclink_ext, "Extensão TipTap: renders links")
    Rel(editor, link_suggestion, "Extensão TipTap: [[ trigger")
    Rel(graph_view, api_js, "GET /api/graph")
    Rel(link_suggestion, api_js, "GET /api/search?q=...")
    Rel(auth_store, api_js, "POST /api/login, /api/logout")
    Rel(doc_store_fe, api_js, "Todos os endpoints /api/documents/*")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## Tabela de responsabilidades — Backend

| Componente | Arquivo(s) | Funções chave |
|---|---|---|
| DocumentStore | `store/documents.go` | Create, GetByID, Update, **UpdateAndSync**, SoftDelete, Restore, Move, ListTree, **ListChildren**, PermanentDelete (FK-safe), EmptyTrash (FK-safe) |
| LinkStore | `store/links.go` | CreateLink, DeleteLink, GetLinksForDocument, **GetGraphData**, **SyncLinksFromHTML** |
| TagStore | `store/tags.go` | ListWithCounts (INNER JOIN), **ListAllWithCounts** (LEFT JOIN), SetDocumentTags, **Update** (nome+cor), **Delete**, **PruneUnused** |
| AttachmentStore | `store/attachments.go` | CreateFile, GetByID, ListByDocument, **ListAllWithDocument**, Delete, **DeleteByDocument**, ListOrphans |
| handlers_documents | `server/handlers_documents.go` | CRUD + **handleListChildren** (`GET /api/documents/{id}/children`) |
| handlers_attachments | `server/handlers_attachments.go` | Upload multipart/octet-stream, **inline Content-Disposition**, override CSP para embed |
| handlers_admin | `server/handlers_admin.go` | Lixeira, backup, **handleAdminListAllTags**, **handleAdminUpdateTag**, **handleAdminDeleteTag**, **handleAdminPruneTags**, **handleAdminListAttachments**, **handleAdminListOrphans** |
| handlers_links | `server/handlers_links.go` | GET/POST/DELETE `/api/documents/{id}/links` |
| handlers_graph | `server/handlers_graph.go` | GET `/api/graph?tag=&all=` |
| handlers_capture | `server/handlers_capture.go` | POST `/api/capture` + Open Graph extraction |
