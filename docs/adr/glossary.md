# Glossário — PKD

## Score de Similaridade (Similarity Score)

Valor numérico float entre 0 e 1 que representa o grau de similaridade semântica entre dois itens (query e documento, ou documento e documento). Calculado como **cosine similarity** entre os vetores de embedding gerados pelo modelo Gemini.

- **1.0**: identidade semântica total
- **0.80+**: alta similaridade (resultado muito relevante)
- **0.65–0.79**: similaridade moderada
- **0.45–0.64**: similaridade mínima aceitável (limite inferior: `semanticQueryFloor = 0.45`)
- **< 0.45**: excluído dos resultados

## Embedding

Representação vetorial de um texto gerada por um modelo de linguagem (Gemini `text-embedding-004`). Documentos são re-embutidos quando seu conteúdo muda (`EmbedStaleDocs`). Armazenados na tabela `document_embeddings` como bytes little-endian de float32.

## Busca Semântica (Semantic Search)

Modalidade de busca que encontra documentos por significado, não por correspondência de palavras-chave. Ativada via `mode=semantic` no parâmetro de query. Limitada a `semanticQueryTopK = 50` resultados. Requer `GEMINI_API_KEY` configurado.

## Busca Léxica (Lexical Search)

Modalidade padrão. Usa FTS5 (SQLite Full-Text Search) com fallback para `LIKE`. Busca por correspondência de termos no título, corpo e tags.

## Flat List (Lista Plana)

Modo de exibição de resultados sem hierarquia pai-filho. Usado em busca semântica para preservar a ordem de relevância. Contrasta com a árvore hierárquica usada na navegação normal.

## Cosine Similarity

Medida de similaridade entre dois vetores não-nulos. Calculada como o produto escalar (dot product) dos vetores normalizados. Range: [0, 1] para vetores de embedding normalizados por L2.

## SemanticHit

Tipo interno Go (`store.SemanticHit`) que associa um `DocID int64` ao seu `Score float32` resultante da busca semântica. Retornado por `SemanticSearchDocIDs`.
