# Planejamento de Evolução — PKD

Registro de ideias de **manutenção evolutiva** do PKD: propostas discutidas e
desenhadas, mas **não implementadas**. Nada aqui é decisão aceita — quando uma
proposta for adotada, ela vira ADR em [`adr/`](adr/), termos vão para o
[glossário](adr/glossary.md), e o acompanhamento da execução vai para
[`proximosPassos.md`](proximosPassos.md).

Formato de cada proposta: contexto, decisões de desenho com alternativas
descartadas, plano de implementação e estado.

---

# 1. API de Escrita Multi-Cliente (`/api/v1/documents`)

**Estado:** proposta desenhada, **não implementada**. Desenhada em 2026-09-05
numa sessão de grilling; nenhum código escrito.

**Uma frase:** transformar `POST /api/import` — hoje um endpoint de cliente único
(o Notas) com token compartilhado — numa API de escrita com identidade por
cliente, aceitando Markdown, com destino padrão `Inbox` e reescrita idempotente
por chave externa.

## 1.1 Contexto

`POST /api/import` (ADR-003) foi desenhado para **um** cliente. Isso está
codificado em três lugares:

- Autenticação por um único segredo estático, `PKD_IMPORT_TOKEN`
  (`internal/config/config.go`), com a rota só existindo quando ele está setado
  (`internal/server/server.go`).
- A tag `notas` aplicada a **todo** documento importado
  (`internal/server/handlers_import.go:77`).
- `content` obrigatoriamente HTML: a conversão de Markdown acontece no cliente
  (o Notas usa `goldmark` — `notas/go.mod`).

Surgiram mais dois clientes com a mesma necessidade — gravar um documento no PKD
sem sessão de usuário:

| Cliente | Situação hoje | Conteúdo |
|---|---|---|
| **Notas** | Integrado e em produção: `notas/internal/handlers/notes.go:422` monta `POST {PKD_URL}/api/import` com `Authorization: Bearer {PKD_TOKEN}`, converte Markdown→HTML com goldmark, manda anexos em base64, checa só o `201` (não lê o corpo). O frontend manda a nota para a lixeira depois (`notas/frontend/assets/js/notes.js:266`) — logo, nunca reenvia. | HTML |
| **meetingLog** | **Nenhuma** integração de saída (`meetingLog/internal/server/server.go:95-140`); `MeetingView.svelte` só tem imprimir. | HTML do TipTap (`Notas *string`) + campos estruturados: `data_hora`, `tipo`, `Participantes`, `Projetos`, `Pautas[]`, `Links[]` (`meetingLog/internal/model/types.go`) |
| **Sessões de agente de código** (OMP / Claude Code) | Inexistente. Motivador original: gravar no PKD relatórios e documentação produzidos durante uma sessão de trabalho, em vez de deixá-los soltos em pastas do disco. | Markdown |

Com três clientes no token único: não se sabe quem escreveu, não se revoga um sem
derrubar os outros, e a tag `notas` mente sobre a origem.

## 1.2 Decisões de desenho

### D1 — Identidade por cliente em `api_clients`, não token compartilhado

Tabela `api_clients` (nome, hash do token, ativo, criado_em, último uso), com
criação e revogação em Administração. O token em claro é exibido **uma única vez**
na criação e guardado como hash — mesmo padrão já usado por `share_links`
(`internal/server/handlers_share.go`).

A identidade é atributo do token, não algo que a requisição declara: um cliente
não pode se declarar outro.

**Alternativa descartada:** um token por variável de ambiente
(`PKD_TOKEN_NOTAS`, `PKD_TOKEN_MEETINGLOG`, ...) — registrar um cliente novo
passaria a exigir editar env var e reiniciar o container, e continuaria sem
revogação granular.

### D2 — Migração auto-provisiona o `PKD_IMPORT_TOKEN` como cliente `notas`

Na primeira subida, se `PKD_IMPORT_TOKEN` estiver setado e não houver cliente
correspondente, cria-se a linha `notas` com o hash desse token. Depois disso a
env var deixa de ser lida.

O Notas em produção continua funcionando **sem alteração nenhuma**, mas passa a
ter identidade e a ser revogável pela UI. Zero janela de quebra e nenhum segundo
mecanismo de autenticação permanente no código.

**Alternativa descartada:** manter `PKD_IMPORT_TOKEN` válido para sempre em
paralelo — perpetua dois caminhos de autenticação e um token sem dono nem
revogação.

### D3 — `POST /api/v1/documents` é canônica; `/api/import` vira shim

Uma implementação de regra de negócio, duas assinaturas de entrada.
`/api/import` mantém o contrato antigo (tag `notas` implícita pela identidade do
cliente, resposta com o `Document` completo) chamando a mesma função de serviço.

O prefixo `/api/v1` marca compromisso de estabilidade: mudança incompatível
exige `/api/v2`, não quebra silenciosa de cliente.

**Alternativa descartada:** espremer o contrato novo dentro de `/api/import` —
obrigaria a preservar peculiaridades do caso Notas (tag fixa, resposta completa)
na rota que os clientes novos usariam.

### D4 — `format: "html" | "markdown"`, conversão canônica no servidor

Default `html` (preserva Notas e meetingLog, que já têm HTML do TipTap).
`markdown` convertido no servidor com `goldmark`, **antes** de
`security.SanitizeEditorHTML` — mesma ordem que ADR-003 já usa para o bloco de
anexos.

Com três clientes, deixar a conversão em cada um significa três conversores
divergentes (GFM, tabelas, quebra de linha, bloco de código): o mesmo Markdown
produziria documentos diferentes dependendo de quem enviou.

Blocos Mermaid funcionam sem tratamento especial: goldmark emite
`<pre><code class="language-mermaid">`, `editorPolicy` já permite `class` em
`code`/`pre` (`internal/security/sanitize.go:43`) e a extensão TipTap
`frontend/src/lib/editor/mermaid-code-block.js` renderiza o diagrama.

**Alternativa descartada:** aceitar só Markdown — forçaria Notas e meetingLog a
um round-trip HTML→Markdown com perda de informação.

### D5 — Tag automática derivada do cliente autenticado

O servidor aplica uma tag com o nome do cliente (`notas`, `meetinglog`, `omp`) e
**acrescenta** as tags enviadas pelo cliente.

Sendo derivada da identidade do token, a tag é inforjável. A tag é o eixo de
organização do PKD e `document_fts` indexa `tags`
(`internal/store/schema.sql`), então "tudo que veio do meetingLog" já é
pesquisável sem UI nova. E o Notas continua ganhando `notas` sem mudar uma linha.

**Alternativa descartada:** coluna `source` no `Document` com selo na UI —
entregaria menos que a tag, ao custo de migração, serialização e componente novo.

### D6 — `external_key` opcional, upsert last-write-wins

Única por `(client_id, external_key)`. Chave conhecida → atualiza o documento e
grava snapshot em `document_versions`; chave nova ou ausente → cria.

Escopo por cliente evita colisão entre `meetinglog:42` e `omp:42`. Sendo
opcional, o Notas não precisa de chave. Título **não** serve como identidade: a
migração deduplica títulos (`internal/store/migrate.go`).

Conflito resolve por **last-write-wins**: a fonte de verdade daquele documento é
a origem externa, e uma edição feita dentro do PKD sobrevive como versão anterior
recuperável.

**Alternativa descartada:** deduplicar por hash do conteúdo — dedupica só o
idêntico, que é exatamente o caso irrelevante: reenvia-se porque mudou.

**Alternativa descartada:** `409` quando o documento foi editado no PKD desde a
última escrita do cliente — exigiria rastrear "última escrita por cliente" contra
`updated_at` e transformaria um pedido trivial numa decisão interativa.

**Gatilho de reversão:** se na prática você passar a editar sistematicamente,
dentro do PKD, documentos que continuam sendo reenviados de fora, o desenho certo
é outro — chave externa só na criação, sem upsert, e o PKD passa a ser dono do
conteúdo depois da primeira escrita.

### D7 — Destino padrão: documento raiz `Inbox`, resolvido por título

Sem `parent_id`, o documento cai num documento de raiz de título `Inbox`, criado
na primeira escrita se não existir. `parent_id` explícito vence o Inbox.

Resolver por título e não por `PKD_INBOX_ID` é deliberado: o ID em homologação
(`pkd2.dalc.in`) não é o ID em produção (`pkd.dalc.in`), e configuração de
cliente por ID quebra exatamente na promoção. Por título, a mesma configuração
funciona nos dois, e apagar o Inbox por acidente é auto-reparável.

Triagem é a operação normal da UI: mover o filho para outro pai e ajustar tags —
que é o motivo de o Inbox ter sido pedido.

### D8 — A API não conhece esquema de cliente

O corpo aceito é
`{title, content, format, tags[], parent_id?, external_key?, assoc_date?, attachments[]}`
e nada mais. Campos estruturados alheios (participantes, pautas, links da
reunião) são renderizados **pelo cliente** dentro do `content` — o meetingLog
sabe que "Pautas" é lista ordenada e que "Links" são âncoras; o PKD receberia
pares de string e produziria algo pior.

Única exceção, porque é campo que o PKD já possui: `assoc_date` opcional
alimenta `assoc_year/month/day`, para o documento cair na linha temporal do
evento (a data da reunião) e não na data da importação.

**Alternativa descartada:** bloco `metadata` chave/valor renderizado pelo PKD —
daria ao PKD opinião sobre como exibir dados que não são dele, e cada cliente
novo quer um cabeçalho diferente.

### D9 — Resposta enxuta `{id, title, url, created}`

`url` montada de `PKD_BASE_URL`; `created` distingue criação de atualização (D6).

O `Document` completo devolveria `body_html` que o cliente acabou de enviar (peso
morto no wire) e tornaria campos internos (`Version`, `Locked`, `Encrypted`,
`AttachmentIDs`) contrato público. O shim `/api/import` mantém a resposta
completa, por compatibilidade.

### D10 — Escopo write-only por enquanto

Clientes só criam e atualizam documentos (estes últimos apenas via
`external_key` própria). A coluna de escopo existe na tabela desde o início para
não exigir migração depois, mas nenhum cliente lê nem lista documentos.

Se um token vazar, o pior caso é lixo criado no Inbox — nunca vazamento do
acervo.

### D11 — Cliente de agente: skill portátil configurada por ambiente

Uma *managed skill* (`pkd-write`) que monta a chamada HTTP, lendo `PKD_URL` e
`PKD_TOKEN` do ambiente da máquina — **os mesmos nomes de variável** que o Notas
já usa (`notas/main.go:152`), um vocabulário só para todos os clientes.

**Alternativa descartada:** servidor MCP — é a evolução natural quando a API
estiver estável, mas adiciona um servidor para manter, versionar e autenticar, e
faz isso mal enquanto o contrato ainda muda. A skill exercita exatamente a mesma
API que o MCP chamaria depois, então não é trabalho descartado.

**Alternativa descartada:** arquivo de config no diretório de usuário
(`~/.pkd.json`) — formato novo, parser novo, e um arquivo de segredo em disco que
backups sincronizam sem aviso.

**Descartado explicitamente:** passar URL/token pelo chat. Token em chat é token
vazado — vira histórico de sessão e memória de longo prazo.

Um cliente por máquina, não um token global: `last_used_at` na tela de
Administração passa a dizer *qual máquina* escreveu, e perder um notebook custa
uma revogação, não a rotação de todos.

## 1.3 Termos propostos para o glossário

Só entram em [`adr/glossary.md`](adr/glossary.md) se a proposta for adotada.

**Cliente de API (API Client)** — Aplicativo externo autorizado a escrever
documentos no PKD sem sessão de usuário. Cada cliente é uma linha em
`api_clients` com nome, token guardado como hash (valor em claro exibido uma
única vez, mesmo padrão de `share_links`) e estado ativo/revogado. A identidade
do cliente é atributo do token, não algo que a requisição declara. Revogar um
cliente não afeta os demais.

**Inbox** — Documento de raiz, de título fixo `Inbox`, destino padrão de tudo que
entra pela API de escrita sem `parent_id` explícito. Resolvido por título e criado
na primeira escrita se não existir. Não é pasta especial no modelo: é um documento
comum agindo como pai, então triar é mover o filho.

**Chave Externa (External Key)** — Identificador que um Cliente de API atribui ao
documento para poder reescrevê-lo depois. Única por `(client_id, external_key)`.
Reenvio com chave conhecida atualiza o documento e grava snapshot em
`document_versions` (last-write-wins). Opcional.

## 1.4 Plano de implementação

Ordem das fases é a de execução. Cada uma é compilável e testável isoladamente;
a fase 6 pode sair depois de o backend estar em `pkd2`. Nada exige deploy
coordenado com o Notas.

### Fase 1 — Persistência

**1.1** `internal/store/schema.sql`, ao final, seguindo o padrão de `share_links`
(linhas 158-165):

```sql
CREATE TABLE IF NOT EXISTS api_clients (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL UNIQUE,
    token_hash   BLOB    NOT NULL UNIQUE,
    scope        TEXT    NOT NULL DEFAULT 'write',
    created_at   TEXT    NOT NULL,
    last_used_at TEXT,
    revoked_at   TEXT
);
CREATE INDEX IF NOT EXISTS idx_api_clients_token_hash ON api_clients(token_hash);
```

Revogação é `revoked_at`, não `DELETE` — mesma semântica de `share_links`.

**1.2** Colunas em `documents`, no slice `colMigrations` de
`internal/store/migrate.go` (linhas 95-118):

```go
{`ALTER TABLE documents ADD COLUMN api_client_id INTEGER REFERENCES api_clients(id)`, "alter documents api_client_id"},
{`ALTER TABLE documents ADD COLUMN external_key TEXT`, "alter documents external_key"},
```

E o índice, junto dos outros `CREATE INDEX` (linhas 120-128):

```go
`CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_external_key
 ON documents(api_client_id, external_key)
 WHERE external_key IS NOT NULL`
```

Índice **parcial**: deixa explícito que documentos criados pela UI não competem
pela chave.

**1.3** `internal/store/api_clients.go` (novo), padrão de `internal/store/shares.go`
(`type XStore struct{ db *sql.DB }` + `NewXStore(db)`):

| Método | Assinatura | Notas |
|---|---|---|
| `Create` | `(name string) (plaintext string, c *model.APIClient, err error)` | `security.NewToken(32)` + `security.HashSHA256`; guarda só o hash; `created_at` RFC3339Nano UTC; nome normalizado (minúsculas, trim) porque virará tag |
| `Provision` | `(name, plaintext string) (*model.APIClient, error)` | Idempotente: no-op se o nome existir. Usado pela fase 2 |
| `LookupByToken` | `(plaintext string) (*model.APIClient, error)` | Varre ativos e compara com `subtle.ConstantTimeCompare`, igual a `ShareStore.LookupByToken` (`shares.go:66`) |
| `TouchLastUsed` | `(id int64) error` | Best-effort; erro ignorado pelo middleware |
| `List` | `() ([]model.APIClient, error)` | Inclui revogados; **nunca** devolve token |
| `Revoke` | `(id int64) error` | `SET revoked_at`; `ErrNotFound` se não existir |

`model.APIClient` (`internal/model/api_client.go`): `ID`, `Name`, `Scope`,
`CreatedAt`, `LastUsedAt *string`, `RevokedAt *string`. **Sem** campo de token.

**1.4** Resolução do Inbox — não existe consulta por título hoje. Em
`internal/store/documents.go`:

```go
func (s *DocumentStore) GetOrCreateRootByTitle(title string) (*model.Document, error)
```

`SELECT ... WHERE parent_id IS NULL AND title = ? COLLATE NOCASE AND trashed_at IS NULL LIMIT 1`;
`sql.ErrNoRows` → `s.Create(nil, title)`.

Cuidado: `Create` desambigua título duplicado para `"Inbox (2)"`
(`documents.go:83-147`). A busca vem antes, então isso não ocorre em operação
normal; duas requisições simultâneas na primeira escrita criariam `Inbox (2)`.
Aceitável (`ponytail: sem lock global; se virar problema, serializar numa
transação de escrita`).

**1.5** Upsert, em `internal/store/documents.go`:

```go
func (s *DocumentStore) FindByExternalKey(clientID int64, key string) (*model.Document, error)
func (s *DocumentStore) MarkExternal(id, clientID int64, key string) error
```

`FindByExternalKey` ignora documentos na lixeira: se você apagou, o próximo
reenvio cria novo em vez de ressuscitar.

### Fase 2 — Migração de compatibilidade

Em `cmd/pkd/main.go`, após `store.Open` e antes de subir o servidor:

```go
if cfg.ImportToken != "" {
    if _, err := store.NewAPIClientStore(db).Provision("notas", cfg.ImportToken); err != nil {
        log.Printf("provision legacy import client: %v", err)
    }
}
```

Fica em `main.go` e não em `migrate.go` porque `store` não importa `config` hoje,
e inverter isso acopla a persistência à configuração por um caso transitório.
`Provision` idempotente cobre reinícios.

`cfg.ImportToken` deixa de ser usado em qualquer outro lugar: marcar como
obsoleto em `internal/config/config.go` e no `README`.

### Fase 3 — Autenticação por cliente

**3.1** `internal/server/middleware_api_client.go` (novo), substituindo
`ImportTokenAuth` (`handlers_import.go:143-154`), que deve ser **removido** junto
com sua referência em `server.go`:

1. Lê `Authorization: Bearer <token>`; ausente/malformado → `401` com
   `WWW-Authenticate: Bearer`.
2. `LookupByToken` → `ErrNotFound` → `401`.
3. `TouchLastUsed` best-effort, fora do caminho crítico.
4. Injeta o cliente no contexto.

Sem *rate limit*: o login tem throttling porque senha é adivinhável; um token de
32 bytes não é.

**3.2** Em `internal/server/server.go`:

```go
r.Group(func(r chi.Router) {
    r.Use(s.APIClientAuth)
    r.Post("/api/v1/documents", s.handleAPIWriteDocument())
    r.Post("/api/import", s.handleImport()) // shim legado (D3)
})
```

A isenção de CSRF já está garantida sem código novo: `middleware_csrf.go:26`
isenta pela **presença do header**
(`strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")`), não pelo
middleware de auth usado.

Remover a condicionalidade `if cfg.ImportToken != ""` que hoje liga a rota:
`/api/import` passa a existir sempre, e quem não tem cliente registrado recebe
`401`.

### Fase 4 — Serviço de escrita

**4.1** `internal/server/write_document.go` (novo) — ponto único de verdade,
consumido pelas duas rotas:

```go
type writeDocumentInput struct {
    Client      *model.APIClient
    Title       string
    Content     string   // HTML ou Markdown, conforme Format
    Format      string   // "html" (default) | "markdown"
    Tags        []string
    ParentID    *int64
    ExternalKey string
    AssocDate   string   // "YYYY-MM-DD", opcional
    Attachments []importAttachment
}

func (s *Server) writeDocument(r *http.Request, in writeDocumentInput) (*writeDocumentResult, int, error)
```

Devolve `(resultado, statusHTTP, erro)` — convenção que `importAttachments` já
usa (`handlers_import.go:96`).

Sequência:

1. **Título**: `TrimSpace`; vazio → `"<Nome do cliente> <data>"` (hoje o import
   usa `"Nota " + timestamp`, `handlers_import.go:47`; generalizar).
2. **Formato**: `html` (default) ou `markdown`; desconhecido → `400`.
3. **Destino**: `ParentID` se informado (validar existência, senão `400`); senão
   `GetOrCreateRootByTitle("Inbox")`.
4. **Localizar existente**: se houver `ExternalKey`, `FindByExternalKey`.
5. **Criar ou reusar**: não encontrado → `docs.Create(parentID, title)`.
6. **Anexos**: `s.importAttachments(r, doc.ID, atts)` e concatenar o bloco
   retornado **antes** da sanitização, exatamente como ADR-003 D2. Falha →
   rollback **somente se o documento foi criado nesta requisição**; num upsert,
   `rollbackImportedDocument` destruiria um documento preexistente — nesse caso,
   devolver o erro sem escrever.
7. **Sanitizar**: `security.SanitizeEditorHTML` + `security.ExtractPlainText`.
8. **Gravar**: `docs.Update(doc.ID, doc.Version, title, safeHTML, plainText, "")`.
   O snapshot em `document_versions` sai de graça — `Update` chama
   `snapshotIfChanged` internamente (`documents.go:209`). Tratar
   `ErrVersionConflict` como `409` mesmo sendo improvável aqui: ignorá-lo
   esconderia escrita concorrente.
9. **Tags**: `append([]string{in.Client.Name}, in.Tags...)` +
   `tags.SetDocumentTags`.
   **Atenção:** `SetDocumentTags` **substitui** todas as tags
   (`tags.go:40-75`) — num upsert, tags adicionadas à mão no PKD são perdidas.
   Coerente com last-write-wins (D6), mas precisa estar comentado no código para
   não parecer bug.
10. **Origem**: `MarkExternal(doc.ID, client.ID, key)` quando houver chave.
11. **Data associada**: parse `2006-01-02` → `docs.UpdateAssocDate`
    (`documents.go:331-343`); inválida → `400`.
12. Recarregar com `GetByID` e devolver.

Não envolver tudo numa transação única: `importAttachments` faz I/O de arquivo e
`store.WithTx` não abrange o backend de storage. Falha no passo 9 deixa o
documento sem tags — comportamento atual do import
(`handlers_import.go:79`, erro de tag é não-fatal).

**4.2** `internal/server/markdown.go` (novo):

```go
var markdownConverter = goldmark.New(
    goldmark.WithExtensions(extension.GFM),
    goldmark.WithRendererOptions(html.WithUnsafe()),
)
```

`extension.GFM` é obrigatório: o caso de uso são relatórios com **tabelas**; sem
ele, tabela vira parágrafo com pipes.

`html.WithUnsafe()` é seguro **aqui e só aqui** porque a saída passa
obrigatoriamente por `SanitizeEditorHTML` no passo 7 — sem ele, HTML embutido no
Markdown seria escapado duas vezes e apareceria como texto literal. Comentar no
código, senão parece falha de segurança numa revisão futura.

Dependência nova: `go get github.com/yuin/goldmark` (o Notas já usa v1.8.2).

**4.3** `internal/server/handlers_api_v1.go` (novo) —
`func (s *Server) handleAPIWriteDocument() http.HandlerFunc`.

Corpo:

```json
{
  "title": "string",
  "content": "string",
  "format": "html | markdown",
  "tags": ["string"],
  "parent_id": 123,
  "external_key": "string",
  "assoc_date": "2026-09-05",
  "attachments": [{"filename": "", "mime_type": "", "data_base64": ""}]
}
```

Resposta `201`:

```json
{"id": 42, "title": "...", "url": "https://pkd.dalc.in/#/doc/42", "created": true}
```

`url` usa o helper `s.baseURL(r)` que já existe (`handlers_share.go:110-117`),
que respeita `PKD_BASE_URL` e cai para o host da requisição quando não
configurado. A rota de documento no frontend é `#/doc/{id}` (`App.svelte:91`);
`Editor.svelte:616` monta o link canônico como `origin + "/#/doc/" + id`.

Erros: `400` (JSON inválido, formato desconhecido, `parent_id` inexistente,
`assoc_date` inválida), `401`, `409`, `413` (anexo acima de `PKD_MAX_*_MB`,
propagado de `importAttachments`), `500`.

**4.4** Shim em `internal/server/handlers_import.go`: `handleImport` decodifica o
corpo antigo, chama `writeDocument` com `Format: "html"`, sem `ExternalKey` nem
`ParentID`, e responde `201` com o **`Document` completo** (comportamento atual
preservado, D9). A tag `notas` deixa de ser hardcoded — vem do nome do cliente
que a fase 2 provisiona. Remover `ImportTokenAuth` e a montagem manual do
documento; `importAttachment`, `importAttachments` e
`rollbackImportedDocument` **permanecem**, usados pelo serviço.

### Fase 5 — Testes

Convenções: `tests/integration/`, `TestMain` com
`store.Open("file::memory:?cache=shared&mode=memory")` e
`httptest.NewServer(srv.Handler())`; helpers `apiPost`/`decodeDoc`
(`tests/integration/documents_crud_test.go`). Requisições com Bearer não precisam
de cookie jar nem CSRF. Provisionar clientes de teste via `Provision` no
`TestMain` (`ImportToken: "test-import-token"` já é o valor usado em
`tests/integration/auth_test.go`).

`tests/integration/api_v1_test.go` — cada teste falha por um bug plausível e
distinto:

| Teste | O que quebra se falhar |
|---|---|
| `TestAPIWriteRequiresToken` | Rota exposta sem auth |
| `TestAPIWriteMarkdownTable` | Conversão GFM ausente (afirmar `<table>` no `body_html`) |
| `TestAPIWriteMermaidSurvivesSanitizer` | Sanitizador remover `class="language-mermaid"` |
| `TestAPIWriteLandsInInbox` | Inbox não resolvido; segunda escrita reusar o mesmo Inbox, não criar `Inbox (2)` |
| `TestAPIWriteExplicitParentWinsOverInbox` | Precedência invertida (D7) |
| `TestAPIWriteAutoTagsClientName` | Tag de origem ausente/forjável mesmo com tags do cliente |
| `TestAPIWriteUpsertByExternalKey` | Duplicação: mesmo `id`, `created` true→false, linha nova em `document_versions` |
| `TestAPIWriteExternalKeyScopedByClient` | Colisão entre clientes |
| `TestAPIWriteAssocDate` | `assoc_*` não gravados; data inválida → `400` |
| `TestImportShimStillTagsNotas` | Regressão do Notas: `201`, `Document` completo, tag `notas` |
| `TestAPIWriteRevokedClientRejected` | Revogação não efetiva |

Verificação manual antes do deploy, com reenvio para provar o upsert:

```bash
jq -Rs '{title:"Relatório", content:., format:"markdown", tags:["infra"], external_key:"caminho/do/arquivo.md"}' arquivo.md \
| curl -sS -X POST "$PKD_URL/api/v1/documents" \
    -H "Authorization: Bearer $PKD_TOKEN" \
    -H 'Content-Type: application/json' --data-binary @- | jq
```

Rodar duas vezes: a segunda deve devolver o mesmo `id` com `created: false`.

### Fase 6 — Administração (frontend)

`frontend/src/lib/components/Admin.svelte` é um componente único com abas em
`activeTab` (`$state('dashboard')`, linha 79). Irmãos: `dashboard`, `tags`,
`attachments`, `external-images`, `protected`, `storage`, `cleanup`, `links`,
`shares`.

1. Nova aba `api-clients` ("Clientes de API"), imitando a seção `shares`
   (linhas ~1523-1553): `div.admin-section` > `div.section-header` com `h3` +
   botões.
2. Endpoints em `internal/server/handlers_admin.go`, no grupo autenticado por
   **sessão** (não o grupo Bearer):
   - `GET /api/admin/api-clients` → lista (sem token)
   - `POST /api/admin/api-clients` `{name}` → `201 {id, name, token}` — única vez
     que o token em claro existe
   - `POST /api/admin/api-clients/{id}/revoke` → `204`
3. Exibição do token no padrão de `ShareDialog.svelte`: `input readonly` + botão
   de copiar via `navigator.clipboard.writeText` com confirmação de 2s (linhas
   48-57), e aviso de que **não será exibido novamente**.
4. Chamadas via `apiGet`/`apiPost` de `frontend/src/lib/api.js`, que já anexam
   `X-CSRF-Token` do cookie `pkd_csrf`.
5. Colunas: nome, criado em, último uso, estado, ação de revogar.
   `last_used_at` é o que responde "esse cliente ainda é usado?" antes de
   revogar.
6. Boxicons e classes `.btn`, `.btn-primary`, `.btn-danger`, `.btn-sm` de
   `frontend/src/styles/app.css`.

Build: `npm run build` em `frontend/`; Dockerfile estágio 1 compila, estágio 2
copia para `internal/server/web/dist/`.

### Fase 7 — Cliente de agente (skill portátil)

Managed skill `pkd-write`, com:

- Gatilhos: "guarda isso no PKD", "salva esse relatório", "documenta isso no PKD".
- Lê `PKD_URL`/`PKD_TOKEN` do ambiente; ausentes → instruir a configuração e
  **não** pedir o token no chat.
- `curl` com `jq -Rs` para escapar o Markdown (evita heredoc e problemas de
  quoting no PowerShell).
- `format: "markdown"` sempre; `external_key` = caminho absoluto do arquivo de
  origem quando existir.
- Responder com a `url` devolvida.
- Nunca ecoar o token nem gravá-lo em arquivo do repositório.

**Instalação em outra máquina:**

1. Registrar um cliente em Administração → Clientes de API (nome sugerido: o host
   da máquina, ex.: `omp-desktop`) e copiar o token exibido uma vez.
2. Definir as variáveis, permanentes para o usuário:

   ```powershell
   [Environment]::SetEnvironmentVariable('PKD_URL','https://pkd2.dalc.in','User')
   [Environment]::SetEnvironmentVariable('PKD_TOKEN','<token>','User')
   ```

   ```bash
   export PKD_URL=https://pkd2.dalc.in
   export PKD_TOKEN=<token>
   ```
3. Validar:

   ```bash
   curl -sS -X POST "$PKD_URL/api/v1/documents" \
     -H "Authorization: Bearer $PKD_TOKEN" -H 'Content-Type: application/json' \
     -d '{"title":"Teste de instalação","content":"# ok","format":"markdown"}' | jq
   ```

   Esperado: `201` e o documento no Inbox. `401` → token errado ou revogado.
4. Promover para produção: trocar `PKD_URL` para `https://pkd.dalc.in` e
   registrar um cliente **novo** nessa instância — tokens não são compartilhados
   entre instâncias.

**Dogfooding:** depois da fase 6, gravar o guia de instalação como documento no
PKD usando a própria API. É o primeiro uso real, valida o caminho completo
(Markdown com tabela e bloco de código, `external_key`, Inbox) e coloca o guia
onde ele vai ser procurado.

### Fase 8 — meetingLog como cliente

Fora do escopo do repositório do PKD; listado para fechar o desenho.

- `POST /api/meetings/{id}/export-to-pkd`, espelhando
  `notas/internal/handlers/notes.go:422`.
- O **cliente** monta o HTML (D8): cabeçalho com data/hora, tipo, participantes,
  projetos, pautas (lista ordenada) e links (âncoras), seguido de
  `reuniao.notas`, que já é HTML do TipTap → `format: "html"`.
- `assoc_date` = `data_hora` da reunião.
- `external_key` = `reuniao/{id}`.
- Anexos (`arquivo`) em base64 no mesmo corpo, como o Notas já faz.
- `PKD_URL`/`PKD_TOKEN` no `.env.example` e no `config.go` do meetingLog.
- Botão em `MeetingView.svelte`, ao lado do de imprimir.

Diferença deliberada em relação ao Notas: o meetingLog **não** apaga nem arquiva
a reunião após exportar — a reunião continua sendo o registro primário; o
documento no PKD é uma cópia.

## 1.5 Checklist para o dia da implementação

- [ ] `PKD_IMPORT_TOKEN` não é mais lido fora do auto-provisionamento.
- [ ] `ImportTokenAuth` removido; nenhuma referência restante.
- [ ] `/api/import` e `/api/v1/documents` isentos de CSRF, e **nenhuma** outra
      rota aceita Bearer.
- [ ] Nenhuma regra de negócio duplicada entre o shim e o handler novo (ambos
      chamam `writeDocument`).
- [ ] Resposta do shim inalterada (`Document` completo) — teste de regressão do
      Notas passando.
- [ ] `SanitizeEditorHTML` aplicado **depois** da conversão de Markdown e da
      concatenação do bloco de anexos.
- [ ] Token em claro aparece **uma** vez, só na resposta de criação; nunca em
      log, nunca em `List`.
- [ ] ADR criado em `adr/` e termos movidos para `adr/glossary.md`.
- [ ] `README.md` e `.env.example`: `PKD_IMPORT_TOKEN` marcado como obsoleto.
- [ ] `CHANGELOG.md` atualizado.
