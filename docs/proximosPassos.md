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
