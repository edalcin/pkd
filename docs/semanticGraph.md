# Prompt: Visualização Semântica no Graph View do PKD

## Contexto

O PKD já possui visualização de grafo (`frontend/src/lib/components/GraphView.svelte`) com toggles independentes para Hierarquia, Links entre docs e Relações com tags. O endpoint `GET /api/graph` (handler em `internal/server/handlers_graph.go`, lógica em `internal/store/links.go` → `GetGraphData`) retorna `{nodes, edges}` onde cada edge tem `edge_type: "hierarchy" | "link" | "tag"`.

O Docker já expõe `GEMINI_API_KEY` como variável de ambiente. O modelo correto para embeddings é `gemini-embedding-001` (endpoint batch: `POST https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:batchEmbedContents`).

## Objetivo

Adicionar um toggle "Semântico" ao Graph View existente. Quando ativado, edges semânticas são sobrepostas ao grafo atual com um visual distinto. A geração de embeddings ocorre no backend (Go), com cache em SQLite para evitar chamadas repetidas à API.

## Implementação

### 1. `internal/config/config.go`

Adicionar campo `GeminiAPIKey string` lido de `os.Getenv("GEMINI_API_KEY")`. Seguir o padrão dos outros campos opcionais já presentes (ex: `ImportToken`).

### 2. `internal/store/migrate.go`

Adicionar migração para criar a tabela de cache de embeddings:

```sql
CREATE TABLE IF NOT EXISTS document_embeddings (
    document_id  INTEGER PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    content_hash TEXT    NOT NULL,  -- SHA-256 de (title || body_text) para invalidação
    embedding    BLOB    NOT NULL,  -- float32 little-endian, 3072 valores = 12288 bytes
    created_at   TEXT    NOT NULL
);
```

### 3. `internal/store/links.go` (ou novo arquivo `internal/store/semantic.go`)

Implementar `GetSemanticEdges(ctx, apiKey string) ([]GraphEdge, error)` com a seguinte lógica:

**a) Carregar docs ativos:**
```sql
SELECT id, title, SUBSTR(body_text, 1, 800) AS body_text
FROM documents
WHERE trashed_at IS NULL AND archived_at IS NULL
```

**b) Cache de embeddings:**
- Para cada doc, calcular `content_hash = sha256(title + "\n" + body_text[:800])`
- Consultar `document_embeddings` para docs com hash diferente do cacheado
- Chamar a API Gemini apenas para os docs sem cache válido (batch de 100)
- Salvar novos embeddings no cache

**c) Chamada à API Gemini (batch):**
```go
// POST https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:batchEmbedContents?key={apiKey}
// Body: {"requests": [{"model": "models/gemini-embedding-001", "content": {"parts": [{"text": "..."}]}}]}
// Response: {"embeddings": [{"values": [float32, ...]}]}
```
Usar `net/http` + `encoding/json` stdlib. Sem nova dependência.

**d) Cosine similarity e arestas:**
- Normalizar todos os vetores (float32)
- Para cada doc, encontrar os top-8 vizinhos com similaridade ≥ 0.60
- Retornar como `[]GraphEdge` com `EdgeType: "semantic"` e `Weight: float32`

**Tipo `GraphEdge` já existente em `links.go` — verificar se já tem campo `Weight float64`. Se não, adicionar sem quebrar serialização JSON existente (usar `omitempty`).**

### 4. `internal/server/handlers_graph.go`

Adicionar handler `GET /api/graph/semantic`:

```go
func (s *Server) handleSemanticGraph() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if s.cfg.GeminiAPIKey == "" {
            http.Error(w, `{"error":"GEMINI_API_KEY not configured"}`, http.StatusServiceUnavailable)
            return
        }
        edges, err := s.links.GetSemanticEdges(r.Context(), s.cfg.GeminiAPIKey)
        if err != nil {
            http.Error(w, "internal error", http.StatusInternalServerError)
            return
        }
        writeJSON(w, http.StatusOK, map[string]any{"edges": edges})
    }
}
```

### 5. `internal/server/server.go`

Registrar a rota no grupo autenticado:
```go
r.Get("/api/graph/semantic", s.handleSemanticGraph())
```

### 6. `frontend/src/lib/components/GraphView.svelte`

**a) Estado:**
```js
let showSemantic = $state(false)
let semanticEdges = $state([])
let semanticLoading = $state(false)
```

**b) Toggle no painel de controles** (junto com os toggles existentes):
```html
<label class="show-all-toggle">
  <input type="checkbox" bind:checked={showSemantic} onchange={toggleSemantic} />
  Semântico
</label>
```

**c) Função `toggleSemantic`** — carrega sob demanda (lazy, uma vez):
```js
async function toggleSemantic() {
  if (showSemantic && semanticEdges.length === 0) {
    semanticLoading = true
    try {
      const data = await apiGet('/api/graph/semantic')
      semanticEdges = data.edges || []
    } catch (e) {
      showSemantic = false
      // exibir mensagem de erro (ex: GEMINI_API_KEY não configurada)
    } finally {
      semanticLoading = false
    }
  }
  // re-renderiza via $effect existente
}
```

**d) `getFilteredData()`** — incluir edges semânticas quando `showSemantic` está ativo:

Dentro do `$effect` existente ou ao montar `setupGraph`, mesclar `rawEdges` com `semanticEdges` filtradas por `showSemantic`. As edges semânticas têm `edge_type: "semantic"`.

**e) Visualização das edges semânticas em `setupGraph`:**

Nas edges com `edge_type === "semantic"`:
- `stroke: "#a78bfa"` (roxo)
- `stroke-dasharray: "3 2"`  
- `stroke-width` proporcional ao `weight` (ex: `0.5 + edge.weight * 2`)
- Classe CSS: `graph-edge--semantic`

**f) Legenda** — adicionar item:
```html
<div class="legend-item">
  <svg width="22" height="10">
    <line x1="1" y1="5" x2="21" y2="5" stroke="#a78bfa" stroke-width="1.5" stroke-opacity=".8" stroke-dasharray="3,2"/>
  </svg>
  <span>Semântico</span>
</div>
```

## Critérios de Aceitação

1. Toggle "Semântico" aparece no controle do grafo somente quando `GEMINI_API_KEY` está configurada (backend retorna 503 sem ela — frontend oculta o toggle se a rota retornar erro).
2. Ao ativar o toggle pela primeira vez, o backend chama a API Gemini, preenche o cache e retorna as edges. Ativações subsequentes usam o cache — resposta imediata, sem chamada à API.
3. Edges semânticas aparecem em roxo tracejado, sobrepostas ao grafo existente, sem substituir ou interferir nas edges existentes (hierarquia, link, tag).
4. Desativar o toggle remove as edges semânticas do grafo sem recarregar.
5. Invalidação automática: se um documento for editado (title/body_text muda), seu hash muda e o embedding é recalculado na próxima ativação do toggle.
6. Nenhuma nova dependência externa no Go ou no frontend.

## Notas

- **Não** adicionar o campo `GEMINI_API_KEY` à documentação pública / `README.md` — a variável já está no Docker e não precisa de nova documentação pública.
- **Não** criar nova página ou rota SPA — apenas adicionar ao grafo existente.
- A tabela `document_embeddings` usa `BLOB` para os vetores (float32, little-endian, 3072 × 4 bytes = 12 KB por doc). Para ~300 docs, o cache ocupa ~3.6 MB no SQLite — aceitável.
- O threshold de similaridade (0.60) e o máximo de edges por nó (8) podem ser constantes no código Go por ora.
- Ao chamar a API em batch, respeitar o limite de 100 documentos por request. Iterar em batches se necessário.
