# Próximos Passos — Migração para `gemini-embedding-2`

> **Implementação de código CONCLUÍDA** em 2026-08-27. As quatro fases estão
> feitas e verificadas. O que resta é ação sua na interface (seção
> "Ação do usuário") mais duas pendências abertas listadas no fim.

**Objetivo:** habilitar `gemini-embedding-2` como opção em Administração →
Preferências → Embeddings Semânticos, com re-embedding completo do corpus.

Decisões registradas em [ADR-004](adr/004-migracao-gemini-embedding-2.md).

## Custo — resolvido, não é fator

`EmbedStaleDocs` trunca o corpo em 800 caracteres, então 283 documentos são
~0,071M tokens.

| Cenário | `001` | `2` | Delta |
|---|---|---|---|
| Re-embed completo | $0,0106 | $0,0142 | **$0,0036** |
| Operação anual (500 edições + 2.000 buscas) | $0,0232 | $0,0310 | **$0,0077/ano** |

Preços verificados em [ai.google.dev/gemini-api/docs/pricing](https://ai.google.dev/gemini-api/docs/pricing):
`001` $0.15/1M, `2` $0.20/1M. Free tier cobre tudo.

## Fases

### Fase 1 — Backend: formatação e troca de modelo ✅

- `internal/store/semantic.go`
  - `isEmbed2`, `embedDocText`, `embedQueryText` — formatação condicional ao modelo
  - `EmbedStaleDocs`: texto do documento via `embedDocText`; `SUBSTR(body_text, 1, ?)`
    agora usa a constante `semanticBodyChars`, que estava **morta** (o SQL
    hardcodava o literal `800`)
  - `SemanticSearchDocIDs`: query via `embedQueryText`; `s.embedModel` lido uma
    única vez para variável local
- `internal/server/handlers_admin.go`
  - `validEmbedModels` = `gemini-embedding-2` + `gemini-embedding-001`
  - `case "embed.model"`: `DELETE FROM document_embeddings` entre `SetEmbedModel`
    e `notify()`

### Fase 2 — Frontend: dropdown ✅

- `frontend/src/lib/components/Admin.svelte` — `EMBED_MODELS` espelha a whitelist:
  duas entradas. Removidos `text-embedding-004` e `embedding-001`, desligados
  pela Google (selecionar qualquer um deles quebrava a busca semântica).

### Fase 3 — Remoção do campo morto `Score` ✅

- `internal/model/document.go` — campo `DocumentTreeNode.Score` removido
- `internal/server/handlers_tree.go` — mapa `scoreByID` e a atribuição removidos

Nenhum componente do frontend lia `score` (`grep -i score frontend/src` → nada);
era peso morto no wire e pista falsa de que havia observabilidade de similaridade.

### Fase 4 — Documentação ✅

- `docs/adr/004-migracao-gemini-embedding-2.md` — **criado**. D1 assimetria,
  D2 prefixo condicional, D3 `DELETE` explícito, D4 3072 dims, D5 default
  inalterado, D6 pisos inalterados. Alternativas descartadas e pré-decididas
- `docs/adr/glossary.md` — *Embedding* corrigido (dizia `text-embedding-004`,
  modelo desligado que nunca foi o default) e agora agnóstico de modelo; novo
  termo **Prefixo de Task** (assimétrico vs. simétrico); *Score de Similaridade*
  corrigido (a tabela de faixas de cor descrevia um badge que nunca existiu na
  UI); *SemanticHit* anotado
- `README.md` — `PKD_EMBED_MODEL` (valores aceitos, `DELETE` na troca) e
  `document_embeddings`
- `docs/semanticGraph.md` — texto embedado e hash, lista de modelos com formato
  de texto, nota sobre os dois modelos desligados

**Não** foi criado `CONTEXT.md`: `docs/adr/glossary.md` já cumpre esse papel.

### Fase 5 — Validação do modelo efetivo no boot ✅

Descoberta ao revisar o deploy: `isValidEmbedModel` guardava **só** a API admin.
`PKD_EMBED_MODEL` (`config.go`) e o valor persistido no DB (`server.go`) entravam
sem checagem — um modelo desligado fazia todo sweep falhar em silêncio, com
`embed.count` em zero e busca só léxica para sempre. E o valor persistido fora da
whitelist deixava o `<select>` do admin **em branco**.

- `internal/config/config.go` — constante exportada `DefaultEmbedModel`, fonte
  única para o default de `NewConfig` e para o fallback
- `internal/server/server.go` — valida `s.cfg.EmbedModel` depois de aplicar o
  valor do DB (único ponto onde os dois caminhos convergem); cai para o default
  com log. Não apaga o valor inválido do DB: fallback é idempotente, e reescrever
  a config do administrador em silêncio é pior

Registrado como ADR-004 D7.

Smoke test executado com o binário real:

```
PKD_EMBED_MODEL=models/text-embedding-004
  → embed: model "models/text-embedding-004" is not supported,
    falling back to models/gemini-embedding-001
  → listening on :18099
PKD_EMBED_MODEL=models/gemini-embedding-2
  → listening on :18098   (sem fallback)
```

## Verificação executada

```
go build ./...                                          limpo
go vet ./...                                            limpo
go test ./tests/... ./internal/...                      contract, integration, unit, store: ok
npm run build (frontend)                                ok (avisos a11y pré-existentes em Editor.svelte)
```

Teste novo: `internal/store/semantic_format_test.go` — `TestEmbedTextFormat`,
6 casos. Cobre o par assimétrico do `2` e a invariante de rollback (todo modelo
que não é o `2` mantém `{título}\n{corpo}` byte-idêntico ao comportamento
anterior). É white-box porque os helpers são unexported — primeiro teste
in-package do repo; os demais vivem em `tests/{unit,contract,integration}`.

Smoke test do fallback de boot executado com o binário real (Fase 5), com os dois
caminhos confirmados no log — modelo desligado cai para o default, modelo novo
sobe sem fallback.

## Ação do usuário (não é código)

1. **Administração → Preferências → Embeddings Semânticos → Modelo →
   `gemini-embedding-2` → Salvar modelo.** Isso apaga os 283 vetores e dispara o
   sweep na hora.
2. Durante o sweep a busca opera só com o léxico — comportamento correto, não
   erro (ADR-002 D1). Acompanhar `Documentos embedados` na mesma tela até voltar
   a 283.
3. Abrir o **Graph View** e comparar a densidade de arestas com a de antes. Só se
   o caráter do grafo mudar, ajustar `semanticSimThreshold` (`semantic.go:26`).
   **Nunca os dois pisos juntos** (ADR-004 D6).

## Pendências abertas

- **`store.SemanticHit.Score` é write-only.** Escrito em `semantic.go`, lido por
  ninguém depois da Fase 3 — a fusão RRF consome só a *ordem* dos hits. Não foi
  removido porque `SemanticHit` é termo do glossário e colapsá-lo para um
  `[]int64` rippla em ADR-002 e no glossário. Documentado como ponto de extensão.
  Decidir se remove.
- **Nenhum teste cobre o `DELETE` da Fase 1.** A degradação para léxico já tem
  teste (`tests/integration/hybrid_search_test.go`), mas não o gatilho
  `PUT /api/admin/settings key=embed.model`. Decidir se vale um teste de
  integração para o handler.

## Follow-up deliberadamente separado

Elevar `semanticBodyChars` (`semantic.go:23`). Hoje 800 caracteres (~250 tokens)
contra um limite de **8.192 tokens** no `gemini-embedding-2` — a maior perda de
qualidade semântica do sistema, bem maior que a diferença entre os dois modelos.
Depois da Fase 1 é a troca de um número. Re-embed completo a ~8.000 tokens/doc:
~$0,45. Ficou isolado para que qualquer mudança de qualidade seja atribuível a
uma causa só, e porque D6 dependeria de um baseline inexistente se o tamanho do
input mudasse junto.
