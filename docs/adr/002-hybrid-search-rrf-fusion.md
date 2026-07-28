# ADR-002: Busca Híbrida por Fusão RRF

**Status:** Aceito
**Data:** 2026-07-28
**Supersede:** [ADR-001](001-semantic-search-score-display.md) (arquitetura de busca semântica isolada)

## Contexto

`GET /api/tree?q=…` tinha dois modos **mutuamente exclusivos**, escolhidos por um `<select>` "Léxica | Semântica" na topbar:

- Sem `mode`: `DocumentStore.listByQuery` fazia `LIKE` em título/corpo/títulos de link externo e devolvia uma árvore hierárquica.
- Com `mode=semantic`: `LinkStore.SemanticSearchDocIDs` fazia cosseno sobre `document_embeddings` e devolvia uma lista plana ranqueada — respondendo `503` quando `GEMINI_API_KEY` não estava configurada.

Problemas: o usuário precisava escolher manualmente qual motor usar (a maioria não sabe quando um documento "merece" busca semântica); a ausência da chave Gemini quebrava a experiência com um erro visível; e não havia forma de combinar os dois sinais de relevância numa única lista.

Referência de desenho: `../newPdfDing/refatoracao/04-busca-hibrida.md`.

## Decisões

### D1 — Fusão automática via Reciprocal Rank Fusion, sem seletor de modo

Toda busca (`q` não vazio) roda os dois recuperadores e funde os candidatos por RRF com `k = 60`:

```go
score[id] += 1.0 / (rrfK + float64(rank+1))  // por lista em que o id aparece
```

Empate de score é desempatado por ID ascendente — obrigatório para determinismo, já que a ordem de iteração de `map` em Go não é estável entre chamadas idênticas.

**Sem `GEMINI_API_KEY`**, ou enquanto nenhum documento estiver embedado, a lista semântica é vazia e o RRF degrada **exatamente** para a ordem léxica — sem `if` distinguindo "híbrida" de "só léxica", sem `503`, sem seletor de modo. `SemanticSearchDocIDs` passou a retornar `([]SemanticHit{}, nil)` quando `apiKey == ""`, em vez de exigir que o chamador faça o gate.

**Alternativa descartada:** manter o seletor e adicionar uma terceira opção "Híbrida" — rejeitada porque a maioria dos usuários não tem informação para escolher entre os três modos, e o objetivo é que a busca simplesmente funcione bem por padrão.

### D2 — Recuperador léxico deixa de ser fallback: FTS5 + LIKE sempre concatenados

`SearchStore.LexicalDocIDs` roda FTS5 primeiro (até 100 candidatos, `ORDER BY documents_fts.rank`) e **sempre**, não só quando o FTS5 devolve zero, roda também o `LIKE` — anexando ao fim os IDs ainda não presentes.

**Por quê:** `documents_fts` indexa apenas `title`, `body_text` e `tags`; o `LIKE` da busca de árvore também casa `document_urls.title`. Um fallback puro (like o antigo `Search` de `/api/search`) perderia silenciosamente esses documentos sempre que o FTS5 devolvesse ≥ 1 linha — regressão em relação ao comportamento anterior de `listByQuery`.

Erro de sintaxe do FTS5 não é propagado (mesma degradação que `Search` já pratica): trata-se como lista vazia e segue para o `LIKE`.

`/api/search` (autocomplete `[[` no editor) **não muda** — continua só léxico via `Search`/`ftsSearch`/`likeSearch` (fallback condicional, debounce de 150ms); uma consulta híbrida ali custaria uma chamada de embedding ao Gemini por tecla.

### D3 — Piso semântico baixo (0.30) e top-k maior (100)

`semanticQueryFloor`: `0.45 → 0.30`. `semanticQueryTopK`: `50 → 100`.

Justificativa: quem decide o peso final de cada resultado é a posição de rank na fusão RRF, não um corte rígido de similaridade. Um candidato semântico fraco entra na lista de candidatos mas já é penalizado pela posição ruim no ranking semântico — não precisa ser excluído antes de chegar ao RRF. Efeito colateral visível: as faixas de cor do badge em `TreeNode.svelte` (0.80 verde / 0.65 âmbar / abaixo laranja) passam a mostrar mais laranja, já que documentos com similaridade fraca agora aparecem.

Consequência arquitetural: `GetSemanticEdges` (arestas do Graph View) usa `semanticSimThreshold = 0.60`, uma constante **separada** — o piso do Graph View não muda.

### D4 — Filtro de view/tags/favoritos aplicado depois da fusão, não em cada perna

`SemanticSearchDocIDs` deixa de filtrar `archived_at IS NULL` na query de vetores (mantém só `trashed_at IS NULL`) — o filtro de `view` passa a ser aplicado **uniformemente** sobre os IDs já fundidos, em `ListByIDsFiltered` (substitui `ListByIDs`), reaplicando as mesmas três cláusulas (`view`, `favoriteOnly`, `tagFilter`) que `listByQuery` tinha.

**Por quê:** a busca no PKD abrange ativos e arquivados (o frontend força `view=all` durante uma busca), e filtrar `archived_at` na query de vetores faria a metade semântica ignorar documentos arquivados que a metade léxica encontra — as duas pernas devem operar sobre o mesmo universo de documentos antes da fusão.

### D5 — Resultado continua lista plana, não árvore

Herdado de ADR-001 D1, agora aplicado a **toda** busca (não só à antiga busca semântica): busca léxica também passa a devolver lista plana em vez de árvore hierárquica, porque a fusão produz um ranking global que uma árvore não consegue expressar.

**Alternativa pré-decidida se a hierarquia durante a busca for considerada indispensável:** manter a lista plana como está e, em `TreeNode.svelte`, exibir o caminho do ancestral como breadcrumb — **não** reintroduzir `buildTree` no caminho de busca, que descartaria a ordem de rank (o bug que ADR-001 já corrigiu para o modo semântico).

## Consequências

- `internal/store/search.go`: `LexicalDocIDs` (+ constante `lexicalCandidateLimit = 100`) e `FuseRRF` (+ constante `rrfK = 60.0`) novos.
- `internal/store/semantic.go`: guarda de `apiKey == ""` movida para dentro de `SemanticSearchDocIDs`; `semanticQueryFloor = 0.30`; `semanticQueryTopK = 100`; query de vetores perde `AND d.archived_at IS NULL`.
- `internal/store/documents.go`: `listByQuery` removida; `ListByIDs` → `ListByIDsFiltered(ids, view, tagFilter, favoriteOnly)`; `ListTree` perde o parâmetro `q` e o ramo de busca.
- `internal/server/handlers_tree.go`: `handleTree` único caminho (`q != ""` → `respondHybridSearch`, senão `ListTree`); `mode` deixa de ser lido — um `?mode=semantic` remanescente de cliente antigo é ignorado, nunca `503`. Erro da perna semântica é logado e não aborta a busca (a léxica sozinha ainda é um resultado útil).
- Frontend: `<select>` de modo, stores `searchMode`/`semanticAvailable` e o fallback de 503 em `loadTree` removidos por completo.
- Testes: `tests/integration/hybrid_search_test.go` (substitui `semantic_filter_test.go`) — degradação sem `GEMINI_API_KEY`, accent-insensitivity via FTS5 (`fotossintese` casa `Fotossíntese`, prova que o léxico agora é FTS5 e não o `LIKE` antigo), `mode=semantic` inerte. `tests/unit/store_rrf_test.go` — contrato puro de `FuseRRF` (fusão, degradação, desempate, limite).
