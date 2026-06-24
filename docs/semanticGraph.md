# Grafo Semântico — Embeddings Proativos no PKD

## Visão geral

O PKD gera automaticamente embeddings de texto para todos os documentos ativos usando a API Gemini, armazena os vetores como BLOBs float32 no SQLite e usa similaridade de cosseno para construir arestas semânticas no Graph View. O processo é **proativo**: os vetores ficam sempre frescos antes de o usuário abrir o grafo.

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
- Carrega docs ativos (`trashed_at IS NULL AND archived_at IS NULL`)
- Hash: `sha256(embedModel + "\x00" + title + "\n" + body[:800])` — inclui o nome do modelo para que uma troca de modelo force re-embed de todos os vetores
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

Vetores de 3072 dimensões (modelo `gemini-embedding-001`) = 12 288 bytes/doc. Para 300 documentos ≈ 3,6 MB no SQLite.

## Configuração

| Variável | Padrão | Efeito |
|---|---|---|
| `GEMINI_API_KEY` | *(desativado)* | Sem ela o worker não roda e `GET /api/graph/semantic` retorna 503 |
| `PKD_EMBED_MODEL` | `models/gemini-embedding-001` | Modelo Gemini; trocar invalida todos os embeddings em cache (re-embed automático) |
| `PKD_EMBED_SWEEP_MINUTES` | `15` | Cadência do sweep de segurança; saves de documentos disparam um sweep imediato adicional |

As configurações são exibidas (somente leitura) em **Administração → Preferências → Embeddings semânticos**.

## Armazenamento de vetores

Driver SQLite: `modernc.org/sqlite` (Go puro, sem CGO). Vetores armazenados como BLOB float32 little-endian — sem extensão `sqlite-vec` ou driver CGO.

Similaridade de cosseno calculada em Go puro:
1. `normalize(v []float32) []float32` — vetor unitário; nil para norma zero
2. `dot(a, b []float32) float32` — produto escalar de vetores já normalizados = cosseno

Complexidade: O(n²·d) onde n = docs ativos, d = dimensões do vetor. Para uso pessoal (centenas de docs) é adequado; para milhares usar ANN (não implementado — YAGNI).

## Fluxo de segurança e consistência

- **Troca de modelo**: hash inclui o nome do modelo → todos os docs ficam stale → re-embed no próximo sweep. Custo único.
- **Doc trasheado/arquivado**: não entra no JOIN; embedding permanece na tabela mas nunca é consultado. FK CASCADE apaga a linha em delete físico.
- **Vetor de norma zero**: pulado silenciosamente (`normalize` retorna nil).
- **Erro de rede/API no worker**: logado com `log.Printf("embedder: %v", err)`, sweep abortado; próximo ticker/notify tenta novamente.
- **Concorrência worker + fetch do grafo**: `embedMu` serializa — fetch espera o sweep terminar e então lê do DB (sem chamada duplicada à API).
- **Sem `GEMINI_API_KEY`**: `EmbedStaleDocs` retorna (0, nil), worker não cria ticker, handler retorna 503.

## Limitações deliberadas (ponytail)

- **Sem busca semântica de texto**: vetores existem; busca é futura — não implementado agora.
- **Sem chat/RAG**: vetores existem para uso futuro.
- **Threshold e max-neighbors fixos**: `semanticSimThreshold = 0.60`, `semanticMaxNeighbors = 8` — constantes no código. Configurar via env var quando houver evidência de necessidade.
- **Batch size fixo**: `semanticBatchSize = 100` — limite prático da API Gemini; não exposto.
- **Ticker não exposto como metrica**: sem endpoint de status do worker além do painel de admin.
