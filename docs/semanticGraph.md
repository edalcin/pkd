# Grafo Semântico — Embeddings Proativos no PKD

## Visão geral

O PKD gera automaticamente embeddings de texto para todos os documentos não trasheados e não protegidos — **arquivados incluídos** — usando a API Gemini, armazena os vetores como BLOBs float32 no SQLite e usa similaridade de cosseno para construir arestas semânticas no Graph View. O processo é **proativo**: os vetores ficam sempre frescos antes de o usuário abrir o grafo. O grafo continua exibindo só docs ativos; o arquivado é embedado para permanecer alcançável pela busca.

Os mesmos vetores também alimentam a **busca híbrida de texto** (`GET /api/tree?q=…`): `SemanticSearchDocIDs` (`semantic.go`, mesmo pacote) fornece a perna semântica, fundida com o recuperador léxico via Reciprocal Rank Fusion. Ver `docs/adr/002-hybrid-search-rrf-fusion.md`. `semanticQueryFloor`/`semanticQueryTopK` controlam só essa busca; `semanticSimThreshold`/`semanticMaxNeighbors` abaixo controlam só as arestas do grafo — constantes independentes no mesmo arquivo.

## Arquitetura

```
Criar/editar doc
       │
       ▼
s.embedder.notify()    (não-bloqueante, coalescente)
       │
       ▼
┌─────────────────────────────────────────────────────────┐
│  embedder goroutine  (inicia com srv.StartEmbedder(ctx)) │
│                                                          │
│  sweep() ─► LinkStore.EmbedStaleDocs(ctx, apiKey)       │
│     ▲                                                    │
│     │  triggers:                                         │
│     ├─ startup (imediato)                                │
│     ├─ notify() channel (save do doc)                    │
│     └─ ticker PKD_EMBED_SWEEP_MINUTES (padrão 15 min)   │
└─────────────────────────────────────────────────────────┘
       │
       ▼
document_embeddings (SQLite BLOB float32 LE)
       │
       ▼
GET /api/graph/semantic
       │
  EmbedStaleDocs() ─► garante vetores frescos (fallback)
       │
  JOIN documents + document_embeddings
       │
  cosseno O(n²), top-8 por nó, threshold 0.60
       │
  []GraphEdge { edge_type: "semantic", weight: float64 }
```

## Componentes

### `internal/store/semantic.go`

**`EmbedStaleDocs(ctx, apiKey string) (int, error)`**
- Carrega docs embedáveis (`embeddableWhere` = `trashed_at IS NULL AND encrypted = 0`) — arquivados incluídos, protegidos fora (corpo é ciphertext)
- **Prune**: DELETE de embeddings cujo `document_id` não é mais embedável (trashed/protegido) — usa a mesma constante `embeddableWhere`, ocorre antes da verificação de stales, a cada sweep
- Texto embedado: `embedDocText(title, body[:semanticBodyChars])` = `title: {t} | text: {b}` (formato assimétrico do `gemini-embedding-2`, ADR-004)
- Hash: `sha256(EmbedModelName + "\x00" + texto)` — inclui nome do modelo; troca de modelo invalidaria todos os hashes (o modelo hoje é fixo)
- Lê hashes cached de `document_embeddings`; calcula stale set
- Chama `embedBatch` em lotes de 100 (limite da API Gemini)
- Upsert em `document_embeddings` via `ON CONFLICT(document_id) DO UPDATE`
- Serializado por `embedMu sync.Mutex` — worker em background e fetch do grafo nunca chamam a API em duplicidade

**`GetSemanticEdges(ctx, apiKey string) ([]model.GraphEdge, error)`**
- Chama `EmbedStaleDocs` como fallback (garante vetores mesmo antes do 1º sweep)
- Lê `(document_id, embedding)` via JOIN para docs ativos com embedding
- Similaridade de cosseno O(n²·d), top-8 vizinhos por nó, threshold 0.60
- Dedup por par canônico (lo, hi); emite `edge_type: "semantic"`

### `internal/server/embedder.go`

Worker goroutine long-lived, estilo `startBackupTempSweep`. Cancela via `ctx` no shutdown (SIGINT/SIGTERM).

```go
type embedder struct {
    links         *store.LinkStore
    apiKey        string
    sweepInterval time.Duration  // PKD_EMBED_SWEEP_MINUTES × time.Minute
    trigger       chan struct{}   // buffer 1 — coalescente
}
```

`notify()` é não-bloqueante: rajadas de saves (import em massa) → único sweep em lote.

### `internal/store/migrate.go` — tabela `document_embeddings`

```sql
CREATE TABLE IF NOT EXISTS document_embeddings (
    document_id  INTEGER PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    content_hash TEXT    NOT NULL,  -- sha256(model + "\x00" + text)
    embedding    BLOB    NOT NULL,  -- float32 little-endian, N × 4 bytes
    created_at   TEXT    NOT NULL
);
```

Vetores de 3072 dimensões = 12 288 bytes/doc. Para 300 documentos ≈ 3,6 MB no SQLite. Ambos os modelos suportados devolvem 3072 dimensões por padrão; o PKD não envia `output_dimensionality`.

## Configuração

### Variáveis de ambiente

| Variável | Padrão | Efeito |
|---|---|---|
| `GEMINI_API_KEY` | *(desativado)* | Sem ela o worker não roda e `GET /api/graph/semantic` retorna 503 |
| ~~`PKD_EMBED_MODEL`~~ | — | **Removida.** O modelo é fixo em `store.EmbedModelName` (`models/gemini-embedding-2`) |
| `PKD_EMBED_SWEEP_MINUTES` | `15` | Cadência do sweep de segurança; saves de documentos disparam sweep imediato adicional |

### Configuração via Admin

**Administração → Preferências → Embeddings semânticos** expõe, tudo somente leitura:
- Status da chave Gemini
- Contagem de documentos embedados
- Modelo em uso (`models/gemini-embedding-2`, fixo — não há troca pela UI nem por env var)

Modelo:
| Modelo | Dimensões | Formato do texto | Notas |
|---|---|---|---|
| `models/gemini-embedding-2` | 3072 | Prefixo de task (assimétrico) | Único modelo. Limite de entrada 8192 tokens |

`models/gemini-embedding-001`, `models/text-embedding-004` e `models/embedding-001` foram removidos: os dois últimos a Google desligou, e o primeiro só existia como rollback de um dropdown que não existe mais.

## Armazenamento de vetores

Driver SQLite: `modernc.org/sqlite` (Go puro, sem CGO). Vetores armazenados como BLOB float32 little-endian — sem extensão `sqlite-vec` ou driver CGO.

Similaridade de cosseno calculada em Go puro:
1. `normalize(v []float32) []float32` — vetor unitário; nil para norma zero
2. `dot(a, b []float32) float32` — produto escalar de vetores já normalizados = cosseno

Complexidade: O(n²·d) onde n = docs ativos, d = dimensões do vetor. Para uso pessoal (centenas de docs) é adequado; para milhares usar ANN (não implementado — YAGNI).

## Fluxo de segurança e consistência

- **Modelo**: fixo em `store.EmbedModelName`. O nome segue dentro do hash de staleness, então uma futura troca da constante ainda invalida 100% dos vetores num único sweep.
- **Doc trasheado/protegido**: embedding é **apagado no próximo sweep** pelo DELETE de prune (`NOT IN (SELECT id FROM documents WHERE trashed_at IS NULL AND encrypted = 0)`). Delete físico do doc é coberto por `ON DELETE CASCADE`. Doc **arquivado** mantém o embedding — sai do JOIN do grafo, mas continua na busca semântica.
- **Vetor de norma zero**: pulado silenciosamente (`normalize` retorna nil).
- **Erro de rede/API no worker**: logado com `log.Printf("embedder: %v", err)`, sweep abortado; próximo ticker/notify tenta novamente.
- **Concorrência worker + fetch do grafo**: `embedMu` serializa — fetch espera o sweep terminar e então lê do DB (sem chamada duplicada à API).
- **Sem `GEMINI_API_KEY`**: `EmbedStaleDocs` retorna (0, nil), worker não cria ticker, handler retorna 503.

## Limitações deliberadas (ponytail)

- **Sem chat/RAG**: vetores existem para uso futuro.
- **Threshold e max-neighbors fixos**: `semanticSimThreshold = 0.60`, `semanticMaxNeighbors = 8` — constantes no código. Configurar via env var quando houver evidência de necessidade.
- **Batch size fixo**: `semanticBatchSize = 100` — limite prático da API Gemini; não exposto.
- **Ticker não exposto como metrica**: sem endpoint de status do worker além do painel de admin.

## Clusters por Comunidade (Graph View)

Quando o toggle **Semântico** é ativado, o frontend detecta comunidades no grafo de similaridade e aplica colorização + layout agrupado.

### Algoritmo: Louvain local-moving (nível único)

**Arquivo**: `frontend/src/lib/graph/community.js`

```js
detectCommunities(nodeIds, edges) → Map<id, communityId>
communityColor(i) → hsl(...)  // ângulo dourado, sem paleta fixa
```

Louvain local-moving sobre grafo não-dirigido ponderado (pesos = similaridade). Nível único — suficiente para KB pessoal. Complexidade O(n·k·iters) onde k = grau médio, máximo 20 iterações.

`communityId` é denso (0..K-1) e derivado da ordem de convergência do Louvain — não é estável entre execuções com grafos diferentes. As cores são atribuídas dinamicamente via ângulo dourado HSL: `hsl((i × 137.508) % 360, 62%, 58%)`.

### Integração com `setupGraph`

1. Constrói `ns` (nodes) e `ls` (links) a partir de `initNodes`/`initEdges` (todos os docs, pois `all=true` é forçado no modo semântico)
2. Extrai subconjunto de arestas `edge_type === 'semantic'`; resolve IDs de fonte/alvo (D3 pode ter objetos ou IDs)
3. Chama `detectCommunities(ns.map(n => n.id), semEdges)` → `idToComm`
4. Atribui `n.community` a cada nó
5. Calcula `clusterCenters`: K centros em posições circulares (raio `min(W,H) × 0.35`)
6. Semeia `n.x`/`n.y` próximos ao centro da comunidade (jitter ±30px)
7. Substitui `forceCenter` por `forceX`/`forceY` (strength 0.18) apontados para o centro da comunidade do nó
8. Popula `communityMeta` (Map<commId, {name, size}>) — singletons (size ≤ 1) excluídos
9. `communityMeta` dispara efeito que inicializa `selectedCommunities` com todos os IDs

### Filtro de comunidades

`selectedCommunities` (`$state(Set)`) controla quais comunidades são visíveis. Alterações aplicam-se via `applyVisibility()`:

```js
select(svgEl).selectAll('.graph-node')
  .attr('display', d => !sel.has(d.community) ? 'none' : null)
select(svgEl).selectAll('line')
  .attr('display', d => sel.has(src.community) && sel.has(tgt.community) ? null : 'none')
```

**Sem restart de simulação**: visibilidade via D3 `display`, não via re-render. O `$effect` de visibilidade é independente do `$effect` principal de render — mudanças em `selectedCommunities` não relançam `forceSimulation`.

`untrack(() => applyVisibility())` ao final de `setupGraph` garante que o filtro persiste entre re-renders (ex: toggle de outro tipo de aresta com Semântico ativo).

### Ponytail: limitações deliberadas

- **Nível único** — sem passes de agregação Louvain. Se clusters ficarem grossos demais (poucas comunidades gigantes), subir `strength` para 0.3 ou adicionar agregação em `community.js`.
- **Front-only** — detecção no browser sobre o subgrafo visível. Vantagem: reage ao filtro de texto e toggles sem roundtrip. Migrar para Go em `GetSemanticEdges` se o número de docs tornar o Louvain pesado (não é o caso para KB pessoal).
- **Nomes de comunidade** — sempre `Comunidade N` (sem IA, sem localStorage). Adicionar quando necessário.
- **Testes**: `frontend/src/lib/graph/community.test.js` — dois casos `node:test` (dois triângulos + singletons); `node --test` sem framework.
