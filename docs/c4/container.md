# C4 Level 2 — Container: PKD

> **Versão**: v2.1 · **Data**: 2026-04-18

## Descrição

PKD é entregue como um único container Docker executando um binário Go que incorpora tanto a API REST quanto o frontend Svelte 5 compilado. Dados persistentes ficam em dois volumes Docker externos ao container.

## Diagrama de Containers

```mermaid
C4Container
    title Container Diagram — PKD

    Person(user, "Usuário", "Acessa via navegador")
    Person_Ext(public, "Visitante público", "Acessa link compartilhado")
    Person_Ext(mobile, "Mobile OS", "Share target via PWA")

    System_Boundary(pkd_system, "PKD Docker Container") {
        Container(go_server, "Go HTTP Server", "Go 1.25, chi router, CGO disabled", "Gerencia autenticação, sessões, CSRF, CSP, rate limiting, todos os endpoints REST, sanitização HTML, extração Open Graph. Expõe porta 8080.")

        Container(svelte_spa, "Svelte 5 SPA", "Svelte 5 + Vite + TipTap v2 + D3.js", "Single-page application compilada em build-time pelo Vite e incorporada ao binário Go via //go:embed. Provê toda a UI: editor com toolbar completa, cards de sub-documentos, grafo, sidebar com tags coloridas, busca, admin, PWA.")
    }

    ContainerDb(sqlite_db, "SQLite Database", "SQLite 3.40+, WAL, FTS5", "Armazena todos os dados: documents, document_links, tags, document_tags, attachments (metadados), share_links. Arquivo único fora do container em volume montado.")

    ContainerDb(att_vol, "Attachments Volume", "Sistema de arquivos do host", "Armazena arquivos binários (imagens, PDFs, etc.) enviados como anexos. Referenciados por metadados no SQLite. Volume montado pelo Docker.")

    Rel(user, go_server, "Requisições HTTP/HTTPS", "Navegador, porta 8080")
    Rel(public, go_server, "Acessa link público", "GET /public/{token}")
    Rel(mobile, go_server, "POST share_target", "POST /api/capture")
    Rel(go_server, svelte_spa, "Serve SPA embutida", "//go:embed web/dist/ em build-time")
    Rel(go_server, sqlite_db, "Lê/escreve dados", "SQL via modernc.org/sqlite (pure Go)")
    Rel(go_server, att_vol, "Lê/escreve arquivos", "os.ReadFile / os.WriteFile")

    UpdateLayoutConfig($c4ShapeInRow="2", $c4BoundaryInRow="1")
```

## Pipeline de Build

```mermaid
flowchart LR
    subgraph Stage1 ["Stage 1: node:22-alpine"]
        A[frontend/src/\n*.svelte *.js] --> B["npm run build\nvite build"]
        B --> C["internal/server/web/dist/\nindex.html + assets/"]
    end

    subgraph Stage2 ["Stage 2: golang:1.25-alpine"]
        C --> D["go build ./cmd/pkd\n//go:embed all:web"]
        D --> E["/out/pkd (binary ~18MB)"]
    end

    subgraph Stage3 ["Stage 3: distroless/static-debian12"]
        E --> F["/pkd (~22MB total image)"]
    end
```

## Responsabilidades dos containers

| Container | Responsabilidade | Tecnologia |
|---|---|---|
| **Go HTTP Server** | Auth, sessões, CSRF, CSP, HSTS, rate limiting, 30+ endpoints REST, sanitização HTML (bluemonday), extração Open Graph (x/net/html), serve SPA embutida com no-cache para index.html | Go 1.25, chi v5, modernc.org/sqlite, bluemonday |
| **Svelte 5 SPA** | Editor TipTap com `[[link]]` autocomplete, toolbar completa (tabela, imagem, alinhamento, destaque), cards de sub-documentos, chips de tag coloridos, Graph View D3.js, sidebar, busca, calendário, admin com gestão de tags/anexos, PWA | Svelte 5, Vite 6, TipTap v2, D3.js (modular) |
| **SQLite Database** | Todos os dados persistentes com ACID, FTS5 para busca full-text, WAL para leituras concorrentes durante backup | SQLite 3.40+, FTS5, WAL, foreign_keys ON |
| **Attachments Volume** | Arquivos binários desacoplados do banco de dados; referenciados por `stored_filename` único na tabela `attachments` | Volume Docker / diretório no host |

## Orçamento de tamanho da imagem (target ≤ 30 MB)

| Componente | Estimativa |
|---|---|
| distroless/static base | ~2 MB |
| Go binary (stripped, no CGO) | ~18 MB |
| Svelte build (TipTap + D3 + app) | ~1.5 MB uncompressed |
| **Total** | **~21.5 MB** |
