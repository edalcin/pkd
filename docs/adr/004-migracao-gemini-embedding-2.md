# ADR-004: Migração para `gemini-embedding-2` e Prefixo de Task Assimétrico

**Status:** Aceito
**Data:** 2026-08-27
**Relacionado:** [ADR-002](002-hybrid-search-rrf-fusion.md) (a degradação de D1 é pré-requisito de D3 aqui)

## Contexto

O PKD embedava documentos com `gemini-embedding-001`, escolhido num dropdown em
**Administração → Preferências → Embeddings Semânticos** cuja whitelist
(`validEmbedModels`) ainda listava `models/text-embedding-004` e
`models/embedding-001` — dois modelos que a Google desligou. Selecionar qualquer
um dos dois invalidava os 283 embeddings (o hash de staleness cobre o nome do
modelo) e então falhava na chamada, deixando a perna semântica vazia até o
administrador voltar atrás.

`gemini-embedding-2` é o modelo atual. Diferenças que importam para o PKD:

| | `gemini-embedding-001` | `gemini-embedding-2` |
|---|---|---|
| Preço texto (paid, standard) | $0.15 / 1M tok | $0.20 / 1M tok |
| Dims default | 3072 | 3072 |
| Limite de entrada | 2.048 tok | 8.192 tok |
| `taskType` | suportado | **deprecado / ignorado** |
| Espaço latente | — | incompatível com o do `001` |

Fonte: [pricing](https://ai.google.dev/gemini-api/docs/pricing),
[embeddings](https://ai.google.dev/gemini-api/docs/embeddings).

**O custo não foi fator de decisão.** `EmbedStaleDocs` trunca o corpo em
`semanticBodyChars` (800 caracteres, ~250 tokens/doc), então 283 documentos são
~0,071M tokens. Um re-embed completo custa $0,0106 no `001` contra $0,0142 no
`2` — delta de **$0,0036**. Em operação (500 edições + 2.000 buscas/ano), o delta
anual é **$0,0077**. O aumento nominal de 33% é irrelevante em valor absoluto, e
o free tier cobre tudo. A decisão é técnica.

## Decisões

### D1 — Prefixo de task assimétrico, um vetor por documento

Para `gemini-embedding-2` a doc deprecia `taskType` e manda pôr a instrução de
tarefa no texto, com formatos distintos por caso de uso. O PKD tem **dois**
consumidores do mesmo vetor armazenado:

- `SemanticSearchDocIDs` — query ↔ documento, **assimétrico**
- `GetSemanticEdges` — documento ↔ documento (arestas do Graph View), **simétrico**

`document_embeddings` tem `document_id` como PK: **um** vetor por documento. Não
é possível satisfazer os dois formatos ao mesmo tempo. Escolhemos otimizar para a
busca:

```go
// documento
"title: " + title + " | text: " + body   // title: none quando vazio
// query
"task: search result | query: " + q
```

O Graph View passa a comparar entre si vetores formatados para retrieval. Está
fora da recomendação da doc para casos simétricos, e é deliberado: a busca é o
único consumidor no caminho crítico do usuário, o Graph View é exploratório e já
tem seu próprio piso (`semanticSimThreshold = 0.60`) para reagir.

**Alternativa descartada:** dois vetores por documento (PK composta
`(document_id, task)` ou coluna extra), um por formato. Rejeitada por dobrar
chamadas de API, dobrar storage (24 KB/doc) e dobrar a lógica de staleness, para
um ganho não medido no consumidor secundário. **Alternativa pré-decidida se as
arestas do grafo piorarem visivelmente:** aí sim o segundo vetor, não um
compromisso no formato da busca.

**Alternativa descartada:** usar o formato simétrico
(`task: sentence similarity | query: {…}`) nos dois lados. A doc diz
explicitamente para não usá-lo em busca ou retrieval.

### D2 — Prefixo condicional ao modelo, não incondicional

`isEmbed2(model)` porteia a formatação. Com `gemini-embedding-001` selecionado,
o texto embedado continua `{título}\n{corpo}` — byte-idêntico ao comportamento
anterior a este ADR.

**Por quê:** os prefixos são convenção do `gemini-embedding-2`; para o `001` a
doc manda usar `taskType` (que o PKD nunca enviou). Aplicá-los ao `001` injetaria
as strings literais `task: search result | query:` como conteúdo semântico,
diluindo o vetor. Mais importante: mantendo o `001` intocado, ele funciona como
**rollback real** — voltar o dropdown restaura exatamente os vetores de antes, e
não um terceiro comportamento nunca testado.

Custo: um `if` em cada um dos dois callers de `embedBatch`, atrás de dois helpers
(`embedDocText`, `embedQueryText`).

### D3 — `DELETE` explícito na troca de modelo, em vez de confiar no hash

O hash de staleness cobre o nome do modelo, então trocar o modelo já invalida
100% dos documentos. Mas `EmbedStaleDocs` faz upsert em lotes de 100
(`semanticBatchSize`) e `SemanticSearchDocIDs` **não** adquire `embedMu` — durante
o sweep, uma busca embeda a query no espaço novo e a compara contra documentos
ainda no espaço antigo. Como D4 mantém 3072 dimensões nos dois modelos, `dot` não
entra em panic; os scores são apenas ruído, e `semanticQueryFloor = 0.30` é baixo
o bastante para deixá-lo passar à fusão.

`handleAdminSetSettings` passa a executar `DELETE FROM document_embeddings` entre
`SetEmbedModel` e `notify()`. A perna semântica fica vazia até o sweep repovoar,
e **ADR-002 D1 já garante** que lista semântica vazia faz o RRF degradar
exatamente para a ordem léxica — sem `503`, sem ramo especial, com teste em
`tests/integration/hybrid_search_test.go`.

Troca "resultados sutilmente errados" por "resultados léxicos corretos", em uma
linha, reusando uma degradação que já existe.

**Alternativa descartada:** `SemanticSearchDocIDs` adquirir `embedMu`. Toda busca
passaria a esperar o sweep inteiro — converte uma janela de ruído numa janela de
latência, pior para o usuário. **Alternativa descartada:** aceitar a janela. Ruído
silencioso é o pior dos três modos de falha: não aparece em log nem em teste.

### D4 — 3072 dimensões; `output_dimensionality` continua não enviado

Os dois modelos devolvem 3072 por padrão e ambos suportam truncar via MRL para
768/1536. Não truncamos.

**Por quê:** com 283 documentos, os 3,3 MB de SQLite e o O(n²·d) de
`GetSemanticEdges` sobre 283 nós não são um problema que existe. E trocar duas
variáveis ao mesmo tempo (modelo + dimensão) impediria atribuir qualquer mudança
de qualidade a uma causa.

**Alternativa pré-decidida** se o Graph View ficar lento em alguns milhares de
documentos: truncar para 768 (4x menos storage, 4x menos CPU no pairwise, ~0,2
ponto de MTEB) é adicionar um campo em `embedBatch` mais um re-embed de meio
centavo. Nota: `gemini-embedding-2` auto-normaliza dimensões truncadas — inócuo
aqui, porque `normalize()` já roda em todo vetor na leitura.

### D5 — Default compilado permanece `models/gemini-embedding-001`

`config.go` não muda. `SettingsStore.EmbedModel()` devolve `""` quando não há
registro e `server.go` só sobrepõe quando não vazio, portanto toda instalação que
nunca tocou o dropdown herda o default compilado. Flipar o default faria essas
instalações re-embedar todo o corpus **no primeiro boot após o upgrade** —
barato, mas é uma chamada de API externa e uma reescrita completa de tabela
disparada por um `docker pull`.

A mudança acontece quando o administrador a escolhe na interface, que era o
requisito.

**Alternativa descartada:** flipar o default e gravar `embed.model = 001` numa
migration para instalações existentes. Seria código novo cujo único trabalho é
impedir uma mudança que ninguém pediu.

### D6 — Pisos de similaridade inalterados; medição pela densidade de arestas

`semanticQueryFloor = 0.30` e `semanticSimThreshold = 0.60` foram ajustados na
distribuição do `001`, e o espaço latente do `2` é outro. Mantemos os dois.

- `semanticQueryFloor` — ADR-002 D3 já argumenta que quem decide o peso é a
  posição no RRF, não este corte; um deslocamento de distribuição é absorvido.
- `semanticSimThreshold` — este é decisivo e **visível**: define quantas arestas o
  Graph View desenha. A contagem de arestas antes/depois é o instrumento de
  medição, sem uma linha de código nova.

Ajustar no máximo um dos dois pisos, nunca os dois juntos.

**Alternativa descartada:** reviver o badge de score na UI para ter
observabilidade permanente. É a feature de ADR-001 que já saiu da interface uma
vez; construir instrumentação permanente para uma medição única não se paga.

## Consequências

- `internal/store/semantic.go`: `isEmbed2`, `embedDocText`, `embedQueryText`
  novos. `EmbedStaleDocs` formata via `embedDocText`; a query de carga passa a
  usar a constante `semanticBodyChars` como parâmetro ligado
  (`SUBSTR(body_text, 1, ?)`) em vez do literal `800` — a constante estava morta.
  `SemanticSearchDocIDs` formata via `embedQueryText` e lê `s.embedModel` uma
  única vez para uma variável local.
- `internal/server/handlers_admin.go`: `validEmbedModels` reduzida a
  `models/gemini-embedding-2` e `models/gemini-embedding-001`;
  `DELETE FROM document_embeddings` no `case "embed.model"`.
- `frontend/src/lib/components/Admin.svelte`: `EMBED_MODELS` espelha a whitelist.
- `internal/model/document.go` e `internal/server/handlers_tree.go`: campo
  `DocumentTreeNode.Score` e o mapa `scoreByID` removidos — nenhum componente do
  frontend lia `score`, era peso morto no wire e uma pista falsa de que havia
  observabilidade de similaridade. `store.SemanticHit.Score` permanece (a fusão
  consome só a ordem dos hits), documentado no glossário como write-only.
- `PKD_EMBED_MODEL` aceita apenas os dois modelos vivos.
- **Não** foi criado `CONTEXT.md`: `docs/adr/glossary.md` já cumpre esse papel
  neste repo, e um segundo arquivo de vocabulário seria convenção paralela.

## Follow-up deliberadamente separado

`semanticBodyChars` continua em 800 caracteres (~250 tokens) contra um limite de
8.192 tokens no `gemini-embedding-2`. Essa é a maior perda de qualidade semântica
do sistema — bem maior que a diferença entre os dois modelos. Foi mantida fora
deste ADR para que qualquer mudança observada na busca seja atribuível a uma
única causa; D6 dependeria de um baseline inexistente se o tamanho do input
mudasse junto. Após D1, elevar o limite é trocar um número; um re-embed completo
a ~8.000 tokens/doc custa ~$0,45.
