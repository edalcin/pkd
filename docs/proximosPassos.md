# Próximos Passos — Nota

> **Concluída e em produção (2026-09-26, `v1.3.0`).** Terceiro tipo de conteúdo do PKD: **Nota**
> (ver glossário: Nota, app Notas). Bloco próprio na barra lateral, igual ao
> da MC. Migração das notas ativas do app Notas (EC2
> `/home/ec2-user/notas`, somente leitura — nunca alterar nada lá).
> Ordem: homologação no UNRAID (`pkd2`, `https://pkd2.dalc.in`, dados em
> `/mnt/user/Storage/appsdata/pkd`) → validação do usuário → produção no EC2,
> migração junto com a atualização do container. Testes: primeiro no Docker
> local (Windows).
>
> **Atenção:** o banco do `pkd2` tem `attachments.backend = s3` (bucket
> `pkd-dev-attachments`), não disco local. A migração usa a API, então grava
> no backend que estiver ativo.

## Estado e próximo passo

1. Feature + migração validadas no Docker local (cópia do banco do `pkd2`):
   74 Notas (201), segunda execução 74×200 (idempotente), 3 favoritas, 10
   anexos, 82 vínculos de tag, `created_at`/`updated_at` originais, cores de
   tag só preenchidas onde faltavam.
2. **Bug pré-existente corrigido:** `/healthz` prendia a única conexão do
   pool; depois da primeira checagem o servidor inteiro travava (aconteceu
   no `pkd2`). Ver CHANGELOG.
3. ~~Implantar no `pkd2` e migrar~~ — feito (2026-09-26, imagem `5ae262d`):
   74 Notas, 3 favoritas, 10 anexos no S3 dev, cores de tag aplicadas,
   `/healthz` estável. Backup anterior:
   `pkd.sqlite.bak-2026-09-26-pre-notas`. Validado pelo usuário, inclusive o
   filtro de tag no bloco Notas (fix `525ecfc`).
4. ~~Produção no EC2~~ — feito (2026-09-26): tag `v1.3.0` → `:stable`
   (digest `sha256:d381ef6f…`, fixado em `/home/ec2-user/docker-compose.yml`).
   Q15: nenhum Documento com tag `notas` coincidia com nota ativa. Migração:
   74 Notas (201), 3 favoritas, 10 anexos no S3 `pkd-prod-attachments`, 82
   vínculos de tag, cores aplicadas só onde faltavam. `/healthz` 200.
   **Backup antes da atualização:** `/home/ec2-user/pkd-backups/2026-09-26-pre-notas/`
   (banco via `.backup`, `docker-compose.yml`, id da imagem anterior, cópia dos
   55 objetos do S3) e `.tgz` do mesmo em
   `C:\Users\EDalcin\Desktop\OMPtemp\pkd-prod-backup\`. Rollback: restaurar a
   linha `image:` do compose salvo e o `pkd.sqlite`.
5. **Próximo (manual, usuário):** desligar o app Notas no EC2 e arquivar o
   repositório `notas`. Depois: item 8 (formas de captura) é prioridade.

**Script de migração** (descartável, fora do repo):
`C:\Users\EDalcin\Desktop\OMPtemp\notas-probe\migrate_notas.py`, com a cópia
somente leitura de `notes.db` e `files/` do EC2 na mesma pasta
(`fetch_files.py` baixa os anexos). Uso:
`PKD_TOKEN=<PKD_IMPORT_TOKEN> python migrate_notas.py --target <url>`
(`--dry-run` lista sem enviar; `--skip 12,34` pula notas). No fim ele imprime
os `UPDATE tags SET color=…` (Q9) para rodar no banco do PKD. Para produção,
copiar de novo `notes.db` e os anexos (as notas mudam com o uso).

## Decisões da Nota (grilling, Q1–Q20)

Não redecidir sem motivo novo.

|#|Decisão|
|---|---|
|Q2|Nota = Documento com tipo, mesma tabela. Sem pai nem filhos; fora da árvore normal; busca, embedding, Chat, Graph View, tags, anexos, links.|
|Q3|Conversão só de ida: Nota → Documento, Nota → Memória.|
|Q4|Bloco Notas: lista plana, favoritas primeiro, depois `created_at` desc. Colapsado por padrão, toggle persistido.|
|Q5|Filtros de tag e favoritos agem no bloco Notas.|
|Q6|Diálogo "+ Nova Nota": só título, depois abre o editor.|
|Q7|Migração: título = 1ª linha não vazia sem `#`; a linha sai do corpo; repetido → "(2)".|
|Q8|Migração: linhas só de hashtags saem do corpo; hashtags viram tags.|
|Q9|Sem tag `notas`. Cor de tag copiada só se a tag não existe ou não tem cor.|
|Q10|Nota fixada (pinned) → Favorita.|
|Q11|Migração mantém `created_at`/`updated_at`; datas explícitas só via Bearer.|
|Q12|Anexos em bloco "Anexos" no fim do corpo (ADR-003 D2).|
|Q13|API `/api/notes`: `POST`, `GET {id}`, `PATCH {id}`; Bearer ou sessão; `idempotency_key` (`notas:<id>`).|
|Q14|Extensão Chrome e share target Android: Fog; prioridade (item 8 abaixo).|
|Q15|Antes de migrar, ler o banco do PKD (somente leitura) e achar notas já exportadas pelo botão antigo (tag `notas`); usuário decide caso a caso.|
|Q16|Ícone padrão `bx-sticky-note`.|
|Q17|Soltar Nota na MC abre diálogo de Data da Memória (ano obrigatório, vazio); cancelar não muda nada.|
|Q18|Soltar Nota na árvore normal converte em Documento na posição, sem diálogo.|
|Q19|Nota arquivada: igual à MC (some da barra lateral; busca e Chat continuam).|
|Q20|Editor TipTap completo, igual ao do Documento.|

Fatos do app Notas (produção, 2026-09-26): 74 notas ativas, 3 arquivadas, 37
na lixeira; 16 tags em uso; 10 anexos (imagens) em notas ativas, 12,6 MB;
corpo em Markdown, sem campo título; anexos não referenciados no corpo.

## O que foi feito

**Backend**
- `internal/store/migrate.go` — colunas `is_note`, `note_key` + índice `UNIQUE`
  parcial em `documents`.
- `internal/store/notes.go` — **novo**. `CreateNote` (idempotência,
  `created_at`/`updated_at` explícitos só honrados pelo handler quando Bearer,
  `favorite`), `GetNote`, `ListNotes` (favoritas primeiro, depois `created_at`
  desc; filtro de tag/favorito), `NoteDocIDs`, guards de hierarquia
  (`ErrNoteHierarchy`), `ConvertNoteToDocument` (reusa `Reorder`, restaura o
  ícone padrão de Documento se ainda era `bx-sticky-note`) e
  `ConvertNoteToMemory` (mesmo retry de colisão de sufixo do `CreateMemory`).
- `internal/store/documents.go` — `Create`/`Move`/`Reorder` rejeitam
  hierarquia com Nota (`ErrNoteHierarchy`); `ListTree`, `RootStats` e
  `listByTags` excluem Notas; `scanDoc`/`scanDocFromTx` devolvem `is_note`.
- `internal/model/document.go` — campo `IsNote` em `Document` e
  `DocumentTreeNode`.
- `internal/server/handlers_notes.go` — **novo**. `POST /api/notes` (aceita
  `created_at`/`updated_at` só quando `Authorization: Bearer`, ignora com
  sessão), `GET`/`PATCH /api/notes/{id}`, `GET /api/notes` (só sessão),
  `POST /api/notes/{id}/convert` (só sessão, `{to:"document",...}` ou
  `{to:"memory",date:...}`). Reusa `tokenOrSession`, `withheldIfEncrypted` e
  `importAttachments`/`rollbackImportedDocument` de `handlers_memories.go`/
  `handlers_import.go`; `reindexNote` mirror de `reindexMemory`.
- `internal/server/handlers_documents.go` — `Create`/`Move`/`Reorder` também
  tratam `ErrNoteHierarchy` como 400.
- `internal/server/handlers_tree.go` — resultados de busca marcam `is_note`
  (mesmo padrão de `is_memory`).
- `internal/server/server.go` — rotas `POST/GET/PATCH /api/notes[/{id}]`
  (`tokenOrSession`) e `GET /api/notes`, `POST /api/notes/{id}/convert` (só
  sessão).

**Frontend**
- `stores/notes.js`, `NewNoteDialog.svelte`, `ConvertNoteToMemoryDialog.svelte`
  — **novos**.
- `Sidebar.svelte` — bloco "Notas" (lista plana, favoritas primeiro) ao lado
  do bloco MC, toggle persistido em `pkd-notes-collapsed` (colapsado por
  padrão), "+ Nova Nota", filtros de tag/favorito também recarregam a lista de
  Notas. Arrastar uma Nota (marcador `application/x-pkd-note` no
  `dataTransfer`, além do id em `text/plain`) para a árvore normal converte em
  Documento na posição solta sem diálogo (Q18); soltar no bloco MC abre
  `ConvertNoteToMemoryDialog` (campos vazios, ano obrigatório) e só converte
  ao confirmar — cancelar não muda nada (Q17).
- `TreeNode.svelte` — Nota em resultado de busca não arrasta, não recebe
  drop, sem "+" (mirror de Memória); `onDrop` converte em vez de
  mover/reordenar quando a origem é uma Nota.
- `Editor.svelte` — sem "Criar sub-documento" em Notas.
- `documents.js`, `Admin.svelte` — recarregam a lista de Notas em
  arquivar/lixeira/restaurar/renomear.

**Docs** — `README.md` (funcionalidade, seção "Nota", modelo de dados,
arquitetura, changelog, `PKD_IMPORT_TOKEN`), `CHANGELOG.md`, glossário (já
continha o termo Nota), `docs/security.md` (Bearer/`tokenOrSession` em
`/api/notes`, `created_at`/`updated_at` só via Bearer), C4 context e
component.

**Verificação**
- `tests/unit/store_notes_test.go` — idempotência, `created_at`/`updated_at`
  explícitos mantidos, ordem do bloco Notas (favoritas primeiro) + filtro de
  tag + filtro de favoritos + exclusão de arquivadas/lixeira, hierarquia
  bloqueada, exclusão da árvore normal, conversão para Documento (sai da
  lista de Notas, aparece na árvore sob o pai, ícone restaurado) e para
  Memória (ID `MEM-…` válido, ano sozinho ok).
- `tests/integration/notes_test.go` — `created_at` honrado com Bearer e
  ignorado com sessão; conversão para Memória com data inválida (31/02) → 400.
- `go test ./tests/... ./internal/...` verde; `go vet ./internal/...` limpo;
  `npm run build` limpo.
- Smoke com binário real (`PKD_LISTEN_ADDR=127.0.0.1:18091`): `POST
  /api/notes` com Bearer + `idempotency_key` + `created_at` + anexo PNG → 201;
  repetir a mesma chave → 200 com o mesmo id e `created_at` mantido; Bearer
  errado → 401; `GET`/`PATCH /api/notes/{id}` ok; `GET /api/tree` sem Notas;
  `GET /api/tree?q=` marca `is_note:true`; `POST /api/notes/{id}/convert`
  para Documento (aparece na árvore sob o pai, some de `/api/notes`) e para
  Memória com `{year:2020}` → `MEM-2020-…`.
- Navegador (via `browser` do eval, sessão logada no smoke acima): bloco
  Notas aparece, expande/colapsa e persiste em `localStorage` após reload;
  "+ Nova Nota" cria e abre o editor; filtro de tag da barra lateral narrows
  a lista de Notas; arrastar uma Nota (evento `DragEvent` sintético com
  `DataTransfer`, já que a instância de Chromium deste ambiente é acessada
  via relay sem foco de janela do SO — CDP não conseguiu sintetizar
  clique/teclado nem screenshot; contornado com `page.evaluate` disparando
  os mesmos eventos DOM que um drag real dispara) para a árvore converte em
  Documento visível sob o pai; arrastar para o bloco MC abre o diálogo de
  Data da Memória sem alterar nada e, ao confirmar o ano, converte e a
  memória aparece no ano correspondente na MC. Sem evidência fotográfica
  (screenshot indisponível neste ambiente); evidência é o estado do DOM lido
  via `page.evaluate` em cada etapa.

---

# Próximos Passos — Memória Cronológica (MC)

> **Concluída, implantada no UNRAID e validada em uso real (2026-09-24).**
> Código em `main` (`14e6978`); Skill do Hermes configurada e funcionando.
> Uma sessão nova deve ler este arquivo,
> [`docs/adr/glossary.md`](adr/glossary.md) (Memória, Memória Cronológica, Data
> da Memória, Período, ID de Memória) e a
> [ADR-007](adr/007-id-publico-de-memoria.md), nessa ordem. Spec original:
> [`docs/memoriaCronologica.md`](memoriaCronologica.md).
>
> A feature anterior (Chat RAG, ADR-006) está concluída e implantada.

## Reinício — faça isto primeiro

1. `git status -sb`: a working tree deve estar limpa e em sincronia com
   `origin/main`.
2. Não há trabalho pendente da MC. Próximo trabalho: revisar "Pontos abertos"
   depois de algumas semanas de uso, ou nova feature pedida pelo usuário.

**Armadilhas conhecidas**

- **Não rode `gofmt -w` em `internal/` inteiro.** Ele reescreve dezenas de
  arquivos sem relação (CRLF → LF e alinhamento). Rode só nos arquivos
  alterados.
- **Smoke test local:** use caminhos nativos do Windows (nunca `/tmp`), por
  exemplo `PKD_DB_PATH=C:/Users/EDalcin/Desktop/OMPtemp/pkdmc/pkd.db`,
  `PKD_ATTACHMENTS_PATH=…/att`, `PKD_PASSWORD`, `PKD_IMPORT_TOKEN`,
  `PKD_LISTEN_ADDR=127.0.0.1:18090`. Compile o binário fora do repo e rode
  `npm run build` antes de `go build`, porque o binário embute `web/dist`.
- Screenshots e binários de teste vão somente para
  `C:\Users\EDalcin\Desktop\OMPtemp`.
- Depois de mudar código, rode `graphify update .`.

## Decisões de desenho (sessão de grilling, Q1–Q17)

Não redecidir sem motivo novo.

|#|Decisão|
|---|---|
|Q1|Memória = Documento com tipo (`memory_id IS NOT NULL`), mesma tabela. Conversão Documento ↔ Memória adiada (sem caso de uso).|
|Q2|Dia da Data da Memória em `assoc_year/month/day`; colunas novas para hora, minuto, Período.|
|Q3|Hermes converte o relato em campos; PKD só valida.|
|Q4|Período no ID = hora de início (almoço `T12`, lanche `T16`, jantar `T18`).|
|Q5|Dentro do dia: sem hora → início → Período mais longo → hora exata → criação.|
|Q6|Anos/meses/dias do mais recente para o mais antigo.|
|Q7|Precisão mínima = ano; nada inventado.|
|Q8|ID nunca muda, mesmo corrigindo a data (ADR-007).|
|Q9|Prefixo fixo `MEM-`; sufixo 6 Crockford aleatório.|
|Q10|`idempotency_key` do cliente com `UNIQUE`.|
|Q11|API: `POST`, `GET {MEM-id}`, `PATCH {MEM-id}`. Sem DELETE, sem busca.|
|Q12|Contrato do `/api/import`: HTML sanitizado + anexos base64; título obrigatório.|
|Q13|Memórias entram no Graph View.|
|Q14|Sem pai nem filhos; bloqueio no backend e no frontend; associação só por link.|
|Q15|Diálogo "+ Nova Memória": Ano/Mês de hoje, Dia vazio e focado.|
|Q16|No editor, Data da Memória substitui Data Associada; ID com botão Copiar.|
|Q17|Ícone padrão `bx-calendar-event` (boxicons, equivalente ao 🗓️).|

## O que foi feito

**Backend**
- `internal/store/migrate.go` — colunas `memory_id`, `memory_hour`,
  `memory_minute`, `memory_period`, `memory_key` + índices `UNIQUE` parciais.
- `internal/store/memories.go` — **novo**. `MemoryDate.Validate` (rejeita
  31/02, hora sem dia, hora + Período…), emissão do ID, `NormalizeMemoryID`
  (Crockford I/L→1, O→0), `CreateMemory` (idempotência + retry de colisão de
  sufixo), `GetByMemoryID`, `UpdateMemoryDate`, `ListMemories` (ordem da
  árvore da MC), `MemoryDocIDs`, guards de hierarquia.
- `internal/store/documents.go` — `Create`/`Move`/`Reorder` rejeitam
  hierarquia com Memória (`ErrMemoryHierarchy` → 400); `ListTree` e
  `RootStats` excluem Memórias; `GetByID` devolve os campos de Memória.
- `internal/server/handlers_memories.go` — **novo**. Rotas e middleware
  `tokenOrSession` (bearer `PKD_IMPORT_TOKEN` com comparação em tempo
  constante, ou sessão). Bearer errado → 401, nunca cai para o cookie.
  `GET /api/memories` (lista da árvore) é só sessão.
- `internal/server/handlers_tree.go` — resultados de busca marcam
  `is_memory`.

**Frontend**
- `stores/memories.js`, `MemoryDateFields.svelte`, `NewMemoryDialog.svelte` —
  **novos**.
- `Sidebar.svelte` — bloco MC entre a árvore e "+ Novo documento", toggle
  persistido em `pkd-mc-collapsed`, anos colapsados por padrão.
- `Editor.svelte` — controle de Data da Memória + ID copiável; sem
  "Criar sub-documento" em Memórias.
- `TreeNode.svelte` — Memória em resultado de busca não arrasta, não recebe
  drop, sem "+".
- `documents.js`, `Admin.svelte` — recarregam a lista da MC em
  arquivar/lixeira/restaurar/renomear.

**Docs** — `README.md` (funcionalidade, seção MC, modelo de dados,
arquitetura, changelog, `PKD_IMPORT_TOKEN`), glossário, ADR-007,
`docs/security.md` (Bearer/`tokenOrSession`), C4 context e component,
[`docs/promptMcHermes.md`](promptMcHermes.md) (prompt da Skill do Hermes).

**Verificação**
- `tests/unit/store_memories_test.go` — validação, formato do ID, idempotência,
  correção de data mantendo o ID, ordem da árvore (cenário da Q5), hierarquia
  bloqueada e exclusão da árvore normal. `go test ./tests/... ./internal/...`
  verde; `npm run build` limpo.
- Smoke com binário real: 201/200 idempotente, 400 em 31/02 e sem título, 401
  com token errado, `GET` com ID minúsculo, `PATCH` de data mantendo o ID,
  sanitização de `<script>`. Navegador: árvore Ano → Mês → Dia na ordem certa,
  correção de data move a Memória para o dia 21, diálogo cria
  `MEM-2026-09-24T18-…` (jantar) e abre o editor, `/api/tree` sem Memórias e
  busca com `is_memory: true`.

## Próximos passos

1. ~~Commit e push~~ — feito (`14e6978`).
2. ~~Deploy no UNRAID~~ — feito; "+ Nova Memória" e árvore da MC validadas.
3. ~~Skill no Hermes~~ — configurada com `docs/promptMcHermes.md`; funcionando.
4. Ordem da árvore (Q6, mais recente primeiro) confirmada pelo usuário em uso
   real — manter.
5. Depois de algumas semanas de uso, revisar os "Pontos abertos" abaixo.
6. **Hermes corrigir Memórias sozinho (sem urgência).** O Hermes disse que
   precisa de um endpoint de edição, mas `PATCH /api/memories/{memory_id}` já
   existe (`server.go`, Bearer ou sessão; seção 4 do prompt). Por enquanto o
   usuário corrige direto no PKD. Investigar, nesta ordem:
   1. A Skill no Hermes tem a versão atual de `docs/promptMcHermes.md`
      (com a seção 4, "Corrija uma Memória")?
   2. O Hermes guarda o `memory_id` das Memórias que cria?
   3. Se o problema for achar o ID de uma Memória que o Hermes não criou ou
      esqueceu: a API não tem busca (Q11). Opção: `GET /api/memories?date=…`
      ou `?q=…` com Bearer — isso redecide a Q11; decidir com o usuário.
7. **Tag de origem configurável no `/api/import` (documentos do Hermes).**
   Hoje `handlers_import.go` força a tag `notas` em todo documento importado
   (`append([]string{"notas"}, body.Tags...)`). Permitir que o chamador defina
   a tag de origem (ex.: `"source_tag": "hermes"`), mantendo `notas` como
   padrão para o app Notas. Objetivo: documentos criados pelo Hermes (skill
   `pkd-documentos`) saírem só com `#hermes`. Por ora o usuário aceita
   `notas` + `hermes`.
8. **PRIORIDADE — formas de captura de Nota (depois da feature Nota).** Quando
   o app Notas for desligado, estas duas formas de criar notas deixam de
   existir. O usuário usa as duas e elas são muito úteis para ele:
   1. **Extensão Chrome** (hoje em `notas/extension/`, com `EXTENSION_TOKEN`)
      → criar Nota no PKD.
   2. **Compartilhamento do Android** (PWA `share_target` em
      `notas/frontend/manifest.json`) → criar Nota no PKD.
   Estão na Fog do mapa wayfinder da feature Nota; decidir depois da migração
   em homologação.

## Pontos abertos (decidir com uso real)

- **Títulos repetidos.** A unicidade de título do PKD vale para Memórias:
  "Almoço em família" vira "Almoço em família (2)". O prompt do Hermes pede
  títulos distintos. Se incomodar, isentar Memórias da regra (afeta
  wikilinks por título).
- **Memórias arquivadas** somem da árvore da MC e não aparecem na visão
  "Arquivados" (que é da árvore normal). Continuam na busca e no Chat.
- **Filtros de tag e favoritos** não se aplicam à árvore da MC.
- **Aglomerado no Graph View** (Q13): se Memórias parecidas poluírem o grafo,
  adicionar filtro mostrar/esconder Memórias.
- **Pré-existente, fora do escopo:** `POST /api/import` não indexa o FTS nem
  notifica o embedder na criação; a nota só entra na busca léxica após um
  restart. A API de Memórias não tem esse defeito.

## Pendências herdadas (Chat RAG)

- Nenhum teste cobre o `DELETE` de embeddings na troca de modelo de embedding.
- `chatRelevanceFloor = 0.50` é um chute; ajustar com uso real (ADR-006 D4).
- Teste automatizado do piso de relevância exigiria injetar servidor Gemini
  falso no `LinkStore`. Decidir se vale.
