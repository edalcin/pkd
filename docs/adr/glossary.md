# Glossário — PKD

## Score de Similaridade (Similarity Score)

Valor numérico float entre 0 e 1 que representa o grau de similaridade semântica entre dois itens, calculado como **cosine similarity** entre vetores de embedding Gemini. Dois pisos distintos usam essa métrica:

- `semanticSimThreshold = 0.60` — arestas semânticas no Graph View (documento ↔ documento).
- `semanticQueryFloor = 0.30` — perna semântica da busca híbrida (query ↔ documento). Baixo de propósito: quem decide o peso final de cada resultado é a fusão RRF por posição de rank, não este corte.

Faixas de cor do badge exibido em `TreeNode.svelte` (independentes do floor de corte):

| Faixa | Cor |
|-------|-----|
| ≥ 0.80 | Verde |
| 0.65 – 0.79 | Âmbar |
| < 0.65 | Laranja |

## Embedding

Representação vetorial de um texto gerada por um modelo de linguagem (Gemini `text-embedding-004`). Documentos são re-embutidos quando seu conteúdo muda (`EmbedStaleDocs`). Armazenados na tabela `document_embeddings` como bytes little-endian de float32.

## Busca Híbrida (Hybrid Search)

Modo único de busca de `GET /api/tree?q=…`: roda sempre o recuperador léxico e o semântico e funde os dois rankings por **RRF**. Substituiu o antigo `mode=semantic` / `mode` ausente (busca léxica) — não existe mais seleção manual de modo. Ver ADR-002.

## RRF (Reciprocal Rank Fusion)

Algoritmo de fusão de rankings: cada item recebe `1 / (k + rank)` de cada lista em que aparece (rank 1-indexado), somado entre listas; ordenação final por score descendente, empate por ID ascendente para determinismo. PKD usa `k = 60` (`store.FuseRRF`). Um item ausente de uma lista simplesmente não recebe contribuição dela — não há erro nem penalidade especial, o que faz a fusão degradar automaticamente para a ordem da lista restante quando a outra está vazia.

## Busca Semântica (Semantic Search)

Perna semântica da busca híbrida: encontra documentos por significado, não por correspondência de palavras-chave, via `SemanticSearchDocIDs`. Limitada a `semanticQueryTopK = 100` resultados acima do piso `semanticQueryFloor = 0.30`. Sem `GEMINI_API_KEY`, retorna lista vazia (não erro) e a fusão RRF degrada para busca puramente léxica.

## Busca Léxica (Lexical Search)

Perna léxica da busca híbrida, via `LexicalDocIDs`: FTS5 (até 100 candidatos, melhor match primeiro) sempre seguido de `LIKE` sobre título/corpo/título de link externo — não é fallback condicional, os dois rodam e são deduplicados e concatenados. Cobre `document_urls.title`, que o índice FTS5 não indexa. Não depende de `GEMINI_API_KEY`.

## Flat List (Lista Plana)

Modo de exibição de resultados sem hierarquia pai-filho, usado por `GET /api/tree?q=…` (busca híbrida) para preservar a ordem do rank fundido. Contrasta com a árvore hierárquica usada na navegação normal (sem `q`).

## Cosine Similarity

Medida de similaridade entre dois vetores não-nulos. Calculada como o produto escalar (dot product) dos vetores normalizados. Range: [0, 1] para vetores de embedding normalizados por L2.

## SemanticHit

Tipo interno Go (`store.SemanticHit`) que associa um `DocID int64` ao seu `Score float32` (cosseno). Retornado por `SemanticSearchDocIDs` — é a entrada da perna semântica na fusão RRF, não o resultado final da busca.
