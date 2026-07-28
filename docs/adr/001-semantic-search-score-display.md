# ADR-001: Exibição de Score na Busca Semântica

**Status:** Superseded por [ADR-002](002-hybrid-search-rrf-fusion.md)  
**Data:** 2026-06-28

> **Nota (2026-07-28):** A busca por texto deixou de ter um `mode=semantic` dedicado — ver ADR-002 para a fusão RRF que substitui a arquitetura de busca semântica isolada descrita abaixo. As decisões D2 (piso e faixas de cor), D3 (reuso de `TreeNode.svelte`) e D4 (campo `Score` com `omitempty`) permanecem válidas; D1 (flat list) passa a valer para toda busca híbrida, não só o modo semântico.

## Contexto

A busca semântica (`mode=semantic`) usa embeddings Gemini + cosine similarity para ranquear documentos. O pipeline atual:

1. `SemanticSearchDocIDs` calcula scores e retorna `[]int64` (scores descartados)
2. `handleTree` chama `buildTree` — usa `map`, perde a ordem do ranking
3. Frontend renderiza árvore hierárquica — sem indicação de relevância

Resultado: busca semântica não respeita o ranking calculado, e o usuário não vê nenhum indicativo de relevância.

## Decisões

### D1 — Flat list ordenada por score

Modo semântico retorna lista plana (sem hierarquia pai-filho), ordenada por score descendente. `buildTree` não é chamado neste modo.

**Alternativas descartadas:**
- Árvore com score: map iteration quebra a ordem; hierarquia e ranking são objetivos conflitantes
- Árvore flat (nós sem filhos mas com estrutura): sem benefício prático vs. lista plana

### D2 — Score: decimal com 2 casas + cor por faixa

Score exibido como `float64` com 2 casas decimais. Faixas de cor no frontend:

| Faixa | Cor |
|-------|-----|
| ≥ 0.80 | Verde |
| 0.65 – 0.79 | Âmbar |
| < 0.65 | Laranja |

Floor efetivo: 0.45 (constante `semanticQueryFloor`).

### D3 — Reusar `TreeNode.svelte`

Score exibido como badge condicional na row existente do `TreeNode`. Sem novo componente. Drag-drop e expand/collapse ficam inativos naturalmente (sem filhos em modo semântico).

### D4 — Campo `Score` em `DocumentTreeNode`

```go
Score float64 `json:"score,omitempty"`
```

Adicionado ao struct compartilhado. `omitempty` garante que respostas normais (não-semânticas) não incluam o campo. Sem novo tipo de resposta.

## Consequências

- `SemanticSearchDocIDs` passa a retornar `([]store.SemanticHit, error)` — pair `(DocID, Score)`
- `handleTree` em modo semântico: constrói flat list direto, sem `buildTree`
- `DocumentTreeNode.Score` presente só em respostas semânticas (omitempty)
- `TreeNode.svelte` mostra badge de score quando `node.score > 0`
- Bug pré-existente de perda de ordem no `buildTree` semântico: corrigido como side-effect
