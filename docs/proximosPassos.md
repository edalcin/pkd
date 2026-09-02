# Próximos Passos — Chat RAG sobre os Documentos

> **Implementação de código CONCLUÍDA** em 2026-09-02 — as cinco fases estão
> feitas e verificadas. O que resta é ação sua no deploy (seção "Ação do
> usuário") mais as pendências abertas no fim.
>
> Documento de handoff entre sessões: uma sessão nova deve ler este arquivo e a
> [ADR-006](adr/006-chat-rag-sobre-documentos.md), nessa ordem.

**Objetivo:** rota `#/chat` que responde perguntas com base nos documentos do
PKD, reusando a busca híbrida como recuperador e a `GEMINI_API_KEY` existente.
Dropdown de **Modelo de Chat** em Administração → Preferências.

Todas as 21 decisões de desenho estão em
[ADR-006](adr/006-chat-rag-sobre-documentos.md) (D1–D12) e os termos em
[glossary.md](adr/glossary.md). **Não redecidir nada** que esteja lá: as
alternativas descartadas e os gatilhos de reversão já foram discutidos.

## Resumo do desenho em uma frase

A pergunta entra na busca híbrida existente (`LexicalDocIDs` +
`SemanticSearchDocIDs` + `FuseRRF`), os até 8 melhores documentos vão
**inteiros** no prompt sob um teto de tokens, o Gemini Flash responde em
streaming SSE, e o backend devolve a lista das fontes que ele mesmo enviou.

## Fases

### Fase 1 — Backend base ✅ (2026-09-02)

- `internal/store/chat.go` — **novo**. Whitelist compilada (`ChatModelFlash` =
  `models/gemini-3.7-flash`, `ChatModelPro` = `models/gemini-3.1-pro` preview,
  `DefaultChatModel`), `IsValidChatModel`, e os helpers compartilhados
  `geminiURL(model, method, sse)` e `setGeminiAuth(req, apiKey)`.
- `internal/store/settings.go` — `ChatModel()` / `SetChatModel()`. Chave
  `chat.model`; **nenhuma migração de schema** (a tabela `settings` já existe).
  Valor fora da whitelist cai para o default sem reescrever o DB (ADR-004 D7).
- `internal/store/semantic.go`
  - `semanticBodyChars` **800 → 20.000** (ADR-006 D2). Dispara re-embed completo
    do corpus no primeiro sweep após o deploy — ver "Ação do usuário".
  - `embedBatch` autentica por header `x-goog-api-key`, não `?key=` (D10).
  - **Bug corrigido:** `SuggestCommunityName` chamava
    `models/gemini-1.5-flash`, família desligada — a sugestão de nome de
    comunidade no Graph View estava morta. Agora recebe `chatModel` e usa o
    Modelo de Chat configurado.
- `internal/server/handlers_admin.go` — `chat.model` exposto em
  `handleAdminGetSettings`; `case "chat.model"` em `handleAdminSetSettings`
  validando pela whitelist (antes, toda chave desconhecida era 400).
- `internal/server/handlers_graph.go` — passa `s.settings.ChatModel()` ao
  `SuggestCommunityName`.
- `tests/unit/store_chat_model_test.go` — **novo**. Fallback com chave ausente,
  round-trip, e modelo fora da whitelist caindo para o default **sem** reescrever
  o valor persistido.

Verificação: `go build ./...` limpo, `go vet ./...` limpo,
`go test ./tests/... ./internal/...` verde.

### Fase 2 — Backend do chat ✅ (2026-09-02)

`POST /api/chat` respondendo `text/event-stream`, dentro do grupo autenticado
(`server.go:258-260`).

- `internal/server/handlers_chat.go` — **novo**.
  - `retrieveChatContext`: reusa `LexicalDocIDs` + `SemanticSearchDocIDs` +
    `FuseRRF` + `ListByIDsFiltered` com `view="all"`; `q` vem de
    `retrievalQuery`, que concatena as 3 últimas mensagens **do usuário** (D6).
  - `chatRelevanceFloor = 0.50` gateando pelo **melhor** `SemanticHit.Score` —
    primeiro consumidor real do campo (D4). Abaixo do piso, responde "não
    encontrei nada relevante" **sem chamar o modelo**.
  - Orçamento: `chatMaxDocs = 8`, `chatTokenBudget = 100_000`, custo por
    `len/4`. Documento que não cabe é **omitido**, nunca truncado (D3).
  - Documentos `encrypted` são excluídos: a perna léxica não os filtra, e o
    corpo é ciphertext.
  - Eventos SSE: `sources` (Documentos Consultados, servidos pelo backend, D5),
    `text` por chunk, `error`, `done`. `chatErrorMessage` traduz 429/400/404 em
    mensagem acionável, **sem retry** (D11).
- `internal/store/chat.go` — `StreamChat`, `ChatDoc`, `ChatMessage`,
  `formatChatDocs`. `systemInstruction` com grounding estrito;
  `generationConfig` só com `maxOutputTokens` — **`temperature` não é enviada**
  (D12). `finishReason` **não** é usado como sinal de fim (pode estar ausente);
  o fim é o corpo fechado. `SAFETY` vira erro. `geminiBaseURL` virou `var` com
  `SetGeminiBaseURLForTest` — hook só para teste, sem caller de produção.
- `tests/integration/chat_test.go` — **novo**. 503 sem `GEMINI_API_KEY`,
  rejeição de corpo vazio/em branco, e rota inacessível sem autenticação.
- `tests/unit/store_stream_chat_test.go` — **novo**. Parser SSE contra frames
  reais (texto, sem parte de texto, frame malformado ignorado, `finishReason`),
  mais as invariantes de contrato: auth por header `x-goog-api-key`, `alt=sse`
  na URL, corpo do documento no request, e `temperature` **ausente**.

Verificação: `go build`/`go vet` limpos, `go test ./tests/... ./internal/...`
verde, 5 testes novos passando.

### Fase 3 — Frontend do chat ✅ (2026-09-02)

- `frontend/src/App.svelte`: `#/chat` em `getRoute()`, `<Chat />` no bloco
  condicional da `content-area`, título "Chat", e o ícone 💬 na topbar (desktop
  e mobile). `chatAvailable` vem de `embed.key_configured`: sem chave o ícone é
  um `<span class="icon-btn-disabled">` com tooltip, **nunca escondido** (D9).
- `frontend/src/lib/components/Chat.svelte` **novo**: `apiFetch` (reusa o CSRF
  de `api.js`, não duplica leitura de cookie) + `ReadableStream` +
  `AbortController`. Parser de frames SSE, cursor de digitação, botão "Parar",
  fontes como links `#/doc/{id}` com `target="_blank"`, e "Salvar como
  documento" via `createDoc` + `saveDoc`.
- `frontend/src/styles/app.css`: `.icon-btn-disabled` (span, não button, para o
  tooltip sobreviver ao `pointer-events`).

**Defeito encontrado no smoke test e corrigido** (mudança de backend, decidida
durante a Fase 3): com a perna semântica falhando — chave inválida, por
exemplo — `retrieveChatContext` devolvia zero documentos e o chat respondia
"não encontrei nada relevante", **mentindo** sobre o corpus quando a verdade é
que a recuperação nunca rodou. `retrieveChatContext` agora devolve `semErr`
separado do erro fatal, o handler emite evento `error`, e `chatErrorMessage`
detecta `API_KEY_INVALID` **antes** do 400 genérico (o Gemini reporta chave
ruim como 400 INVALID_ARGUMENT, e a mensagem de "excedeu o limite" mandaria o
usuário caçar o problema errado).

Verificação — smoke test com o binário real e navegador:

|Caminho|Resultado|
|---|---|
|Sem `GEMINI_API_KEY`|`POST /api/chat` → 503; ícone desabilitado, opacidade 0.35, tooltip correto|
|Chave inválida + documentos|SSE `sources: []` + `error` com a mensagem de chave inválida, em 1,2 s|
|Piso de relevância (base sem embeddings)|"Não encontrei nada relevante sobre isso nos seus documentos."|
|Salvar como documento|Documento criado e visível em `GET /api/tree`|
|Bolha vazia em erro|Removida (`messages.slice(0,-1)` quando o texto ficou vazio)|

`npm run build` limpo; `go test ./tests/... ./internal/...` verde.

**Nota de ambiente para sessões futuras:** rodar o binário com
`PKD_DB_PATH` em `/tmp` (filesystem do sandbox) faz o SQLite **pendurar** no
primeiro `INSERT` — com `SetMaxOpenConns(1)` (`migrate.go:30`) a conexão única
fica presa e todo o app trava, inclusive `/healthz`. Não é bug do PKD: usar
sempre um caminho nativo do Windows no smoke test.

### Fase 4 — Dropdown no admin ✅ (2026-09-02)

`Admin.svelte`, aba Preferências, nova seção **"Chat com os documentos"** logo
abaixo de "Embeddings semânticos":

- `CHAT_MODELS` espelha a whitelist de `internal/store/chat.go`, com o rótulo
  `(preview)` explícito no `gemini-3.1-pro` (D9).
- `chatModel` é carregado de `chat.model` no mesmo `apiGet('/api/admin/settings')`
  que já roda no mount. Como o servidor reporta o modelo **efetivo** (cai para o
  default quando o persistido saiu da whitelist), o `<select>` nunca aparece em
  branco — o defeito que ADR-004 D7 documenta.
- `saveChatModel` usa o payload genérico existente
  `apiPut('/api/admin/settings', {key:'chat.model', value})`.
- Texto explicando que trocar o Modelo de Chat **não invalida nada**, em
  contraste com o Modelo de Embedding.

Verificação — navegador contra o binário real:

|Caminho|Resultado|
|---|---|
|Opções do `<select>`|exatamente os dois modelos da whitelist, com rótulos|
|Valor inicial|`models/gemini-3.7-flash` (default, não em branco)|
|Salvar `Pro`|`Salvo!` e `GET /api/admin/settings` → `models/gemini-3.1-pro`|
|Reload da aba|`<select>` volta em `models/gemini-3.1-pro`|
|`PUT` com `models/gemini-1.5-flash`|400 `chat.model is not a supported model`; valor anterior intacto|

### Fase 5 — Testes e documentação ✅ (2026-09-02)

- `internal/server/handlers_chat_test.go` — **novo**, primeiro teste in-package
  do pacote `server`. `TestRetrievalQuery`: a query de follow-up carrega o
  assunto do turno anterior, turnos do modelo **não** entram (uma resposta longa
  afogaria a query), o limite de `chatHistoryTurns` é respeitado, e turnos em
  branco não vazam. `TestChatErrorMessage`: trava a **ordem** dos casos —
  `API_KEY_INVALID` antes do 400 genérico, senão o usuário é mandado caçar o
  problema errado.
- `README.md`: linha de feature do Chat na tabela; `GEMINI_API_KEY` passa a
  listar os **três** consumidores (embeddings, Chat, sugestão de nome de
  comunidade); nova seção "Chat com os documentos" com o caminho completo de uma
  pergunta; `settings` na tabela de schema agora cita `chat.model` e registra
  que o modelo de embedding **não** vive lá.
- `docs/semanticGraph.md`: `semanticBodyChars` = 20.000 (era 800), auth por
  header, e a seção de admin do Modelo de Chat.

**Documentação corrigida de quebra** (`README.md:214`): o README descrevia um
badge de similaridade de cosseno com faixas de cor ao lado dos títulos de
busca. Esse badge **nunca existiu na interface** — `glossary.md:10-14` já
registrava isso, e o campo `score` saiu do wire na ADR-004. A linha foi
substituída pelo comportamento real.

Sobre o teste de integração do piso de relevância que esta fase previa: ele
exigiria uma `GEMINI_API_KEY` **válida**, porque sem chave
`SemanticSearchDocIDs` retorna vazio antes de qualquer piso. O caminho está
coberto por smoke test manual (Fase 3) e pelos testes de contrato de
`StreamChat` com servidor falso. Um teste automatizado real exigiria injetar um
servidor Gemini falso no `LinkStore`, que hoje constrói seu próprio
`http.Client` — refatoração maior que o teste. **Decidir se vale.**

## Ação do usuário depois do deploy (não é código)

1. O novo `semanticBodyChars` invalida os 283 vetores (o hash de staleness cobre
   o texto embedado). O sweep repovoa automaticamente; custo ~$0,45.
2. Durante o sweep a busca opera **só com o léxico** — comportamento correto,
   não erro (ADR-002 D1). Acompanhar `Documentos embedados` em Administração →
   Preferências até voltar a 283.
3. Depois do sweep, conferir a densidade de arestas do **Graph View**. Os
   vetores agora representam 20.000 caracteres em vez de 800, então a
   distribuição de similaridade muda. Se o caráter do grafo piorar, ajustar
   `semanticSimThreshold` (`semantic.go`) — **nunca os dois pisos juntos**
   (ADR-004 D6).
4. Sugestão de nome de comunidade no Graph View voltou a funcionar (usava um
   modelo desligado). Vale testar.

## Pendências herdadas, ainda abertas

- **Nenhum teste cobre o `DELETE` de embeddings na troca de modelo de
  embedding.** Herdado da migração anterior; decidir se vale.
- **`chatRelevanceFloor = 0.50` é um chute.** O valor certo só sai de uso real.
  É constante, não configuração de UI (ADR-006 D4).
