# Glossário — PKD

## Score de Similaridade (Similarity Score)

Valor numérico float entre 0 e 1 que representa o grau de similaridade semântica entre dois itens, calculado como **cosine similarity** entre vetores de embedding Gemini. Dois pisos distintos usam essa métrica:

- `semanticSimThreshold = 0.60` — arestas semânticas no Graph View (documento ↔ documento).
- `semanticQueryFloor = 0.30` — perna semântica da busca híbrida (query ↔ documento). Baixo de propósito: quem decide o peso final de cada resultado é a fusão RRF por posição de rank, não este corte.

O score não é exibido na interface. `TreeNode.svelte` nunca renderizou o badge de
faixas de cor previsto em ADR-001, e o campo `score` da resposta de
`GET /api/tree` foi removido por ser peso morto no wire. Para observar a
grandeza que `semanticSimThreshold` controla, use a densidade de arestas do
Graph View.

## Embedding

Representação vetorial de um texto gerada por um modelo Gemini de embedding.
Documentos são re-embutidos quando seu conteúdo **ou o modelo** muda
(`EmbedStaleDocs`, cujo hash cobre os dois). Armazenados na tabela
`document_embeddings` como bytes little-endian de float32, 3072 dimensões.
Vetores de modelos diferentes vivem em espaços latentes distintos e não são
comparáveis entre si — trocar o modelo exige re-embedar todo o corpus.

## Prefixo de Task (Task Prefix)

Instrução de tarefa embutida no próprio texto enviado ao `gemini-embedding-2`,
que substitui o parâmetro `taskType` (deprecado nesse modelo). Duas famílias:

- **Assimétrica** — query e documento recebem formatos *diferentes*: a query vira
  `task: search result | query: {q}`, o documento vira `title: {t} | text: {b}`.
  É o caso da **Busca Híbrida**.
- **Simétrica** — os dois lados recebem o *mesmo* formato. É o caso natural das
  arestas do **Graph View** (documento ↔ documento).

O PKD guarda **um** vetor por documento, formatado para o caso assimétrico; o
Graph View reusa esse vetor. Ver ADR-004. Para `gemini-embedding-001` nenhum
prefixo é aplicado, preservando o formato histórico `{título}\n{corpo}`.

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

Tipo interno Go (`store.SemanticHit`) que associa um `DocID int64` ao seu `Score float32` (cosseno). Retornado por `SemanticSearchDocIDs` — é a entrada da perna semântica na fusão RRF, não o resultado final da busca. A fusão consome apenas a *ordem* dos hits, então `Score` hoje é escrito e nunca lido; permanece como ponto de extensão caso a exibição de score volte.
