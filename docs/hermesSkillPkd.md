# Skill "pkd" do Hermes — texto para colar

Cole o bloco abaixo, inteiro, no chat do Hermes (Telegram ou Web UI). O Hermes
grava a Skill, apaga as Skills antigas e testa o acesso ao PKD sem criar nada.

Requisitos no Hermes: os scripts `/opt/data/bin/pkd_note.py`,
`pkd_memory.py` e `pkd_doc.py`, e o token `PKD_MEMORIES_TOKEN` em
`/opt/data/.env`. No Cloudflare Access, `pkd.dalc.in` precisa de bypass para
`api/memories`, `api/notes` e `api/documents`; sem ele, o `ping` responde `302`.

Contrato da API: [`promptHermes.md`](promptHermes.md).

---

~~~~text
Crie (ou substitua) a skill local "pkd" na categoria "research".
Grave o conteúdo abaixo, exatamente como está, em
/opt/data/home/.agents/skills/research/pkd/SKILL.md
(crie a pasta se não existir). Depois:
1. Apague as skills antigas "pkd-documentos" e "pkd-memorias", se existirem.
2. Confirme que estes scripts existem e são executáveis:
   /opt/data/bin/pkd_note.py, /opt/data/bin/pkd_memory.py, /opt/data/bin/pkd_doc.py
3. Rode os três "ping" (pkd_note.py ping, pkd_memory.py ping, pkd_doc.py ping) e me diga o resultado de cada um. Não crie nenhum conteúdo de teste no PKD.
4. Me mostre "hermes skills list" filtrado por "pkd".

CONTEÚDO DO SKILL.md:

---
name: pkd
version: 1.0.0
author: Eduardo Couto Dalcin & Hermes
license: MIT
description: "Use when criar Nota, Memória ou Documento no PKD."
metadata:
  hermes:
    tags: [pkd, notas, memorias, memoria-cronologica, documentos, registro]
    related_skills: [pkd-consulta, cloudflare-access]
---
# Criar conteúdo no PKD

O PKD é a base de conhecimento pessoal do Eduardo (`https://pkd.dalc.in`).
Você cria nele três tipos de conteúdo: **Nota**, **Memória** e **Documento**.
Esta skill substitui `pkd-memorias` e `pkd-documentos`. Para ler ou buscar
no PKD, use `pkd-consulta`.

Fonte do contrato: `/Storage/git/pkd/docs/promptHermes.md` e
`internal/server/server.go` no mesmo repositório.

## When to Use

- O Eduardo pede para guardar algo **no PKD**: "crie uma nota no PKD…",
  "crie uma memória no PKD…", "crie um documento no PKD…", "registra / anota
  / salva / guarda no PKD…".
- Ele corrige uma Memória que você criou ("na verdade foi no dia 22"): §4.3.

Não use quando o pedido não cita o PKD (ex.: só "cria uma memória"). Isso é um
pedido para a sua própria memória do Hermes. Na dúvida, pergunte.

## 1. Escolha o tipo

| Tipo | O que é | Exemplos | Utilitário |
|---|---|---|---|
| **Nota** | Informação curta e rápida, sem data de evento | endereço, telefone, lista de compras, ideia solta | `pkd_note.py` |
| **Memória** | Registro de um **evento** que já aconteceu | almoço em família, consulta médica, entrega, visita | `pkd_memory.py` |
| **Documento** | Texto organizado e duradouro, com seções | resumo de pesquisa, roteiro, receita, procedimento | `pkd_doc.py` |

- O Eduardo disse o tipo ("nota", "memória", "documento")? Use esse tipo.
- "crie no PKD…" sem tipo: escolha pela tabela. Se a dúvida continuar entre
  dois tipos, pergunte em uma linha ("Nota ou Documento?").
- Tarefa ou lembrete futuro não é Memória.

## 2. Regras comuns

- **Título:** o que o Eduardo disser; se não disser, gere um curto e
  específico. O PKD acrescenta " (2)" se o título já existir.
- **Ditado** ("crie no PKD: …"): texto fiel, só formatado. **Pedido de
  redação** ("um documento sobre X", "resuma esta conversa"): você redige.
  Não acrescente fatos que o Eduardo não disse.
- **Formato:** HTML simples, em português. Nada de Markdown (aparece cru).
  Tags permitidas: `<p> <h2> <h3> <strong> <em> <ul> <ol> <li> <a href>
  <blockquote> <code> <pre> <table> <tr> <th> <td>`. O PKD sanitiza.
- **Tags:** Nota e Documento recebem só `hermes`, que o script põe sempre.
  Não envie outras tags, mesmo que o Eduardo cite uma (ele acrescenta na
  interface). Memória: **sem tags** (o script recusa).
- **Anexos:** arquivos do chat ou caminho local vão com `--attach CAMINHO`
  (Nota e Documento, repetível) ou em `attachments` base64 (Memória). Se um
  anexo falhar (ex.: `413`), nada é criado.
- **Idempotência:** o script gera uma `idempotency_key` e a grava de volta no
  arquivo do payload. Em timeout ou erro de rede, rode de novo sobre o
  **mesmo arquivo**: o PKD devolve o item que já existe (`200`), sem duplicar.
- O pedido do Eduardo já autoriza o envio: **não peça confirmação**.
- O token é o `PKD_MEMORIES_TOKEN` em `/opt/data/.env` (o mesmo
  `PKD_IMPORT_TOKEN` do servidor). Nunca o escreva em chat, skill ou payload.

Respostas comuns: `201` criado; `200` a chave já foi usada (não é erro);
`400` dado inválido (o corpo diz o motivo: corrija ou pergunte); `401` token
errado (avise, não repita); `413` anexo grande demais; `302` para
`edalcin.cloudflareaccess.com` = falta bypass no Cloudflare Access (ver
Pitfalls).

Depois de criar, confirme em uma linha: tipo, título e o `LINK` que o script
imprime (`https://pkd.dalc.in/#/doc/{id}`).

## 3. Crie uma Nota

```bash
# payload em $TMPDIR/pkd_nota_<slug>.json: {"title": "...", "content": "<p>...</p>"}
/opt/data/bin/pkd_note.py create $TMPDIR/pkd_nota_<slug>.json [--attach /caminho/arq] --dry-run
/opt/data/bin/pkd_note.py create $TMPDIR/pkd_nota_<slug>.json [--attach /caminho/arq]
/opt/data/bin/pkd_note.py ping     # acesso + token, sem criar nada (espera 400)
/opt/data/bin/pkd_note.py log      # Notas criadas (/opt/data/pkd_notas.jsonl)
```

A Nota aparece no Mural de Notas do PKD, fora da árvore.

## 4. Crie uma Memória

### 4.1 Descubra a Data da Memória

Converta o relato em campos, com a data e a hora **locais do Eduardo**
(America/Sao_Paulo) no momento do relato.

- `year` é obrigatório. `month`, `day`, `hour`, `minute` e `period` só entram
  quando o relato permite saber. **Nunca invente** um componente: "em 1998
  fiz um mochilão" → só `year: 1998`.
- Resolva datas relativas: "ontem", "anteontem", "sábado passado", "semana
  passada na quinta".
- Relato feito entre 00:00 e 05:59 com "ontem" ou "hoje" é ambíguo: pergunte
  qual dia antes de criar.
- Hora exata → `hour` (0–23) e, se souber, `minute` (0–59).
- Momento vago → `period` (use **ou** `hour` **ou** `period`, nunca os dois):

  | Relato | `period` |
  |---|---|
  | de madrugada (00–06h) | `madrugada` |
  | de manhã (06–12h) | `manha` |
  | no almoço, na hora do almoço (12–15h) | `almoco` |
  | à tarde (12–18h) | `tarde` |
  | no lanche, no café da tarde (16–18h) | `lanche` |
  | no jantar (18–21h) | `jantar` |
  | à noite (18–24h) | `noite` |

- Hora ou período exigem `day`; `day` exige `month`.

Exemplo — relato em 24/09/2026 às 15h: *"ontem almocei com minhas irmãs,
Bruna e Claudia, no Rascal do Leblon"* →
`{"year": 2026, "month": 9, "day": 23, "period": "almoco"}`.

### 4.2 Escreva e envie

- `title`: sem data (a data já está nos campos). Ex.: "Almoço de aniversário
  da Bruna no Rascal".
- `content`: registre quem, onde, o quê e por quê, como o Eduardo relatou.
- **"Fomos" / primeira pessoa do plural sem sujeito explícito** (fomos,
  jantamos, viajamos…): assuma **Eduardo e Claudia (esposa)** e escreva os
  dois nomes no texto ("Eduardo e Claudia foram ao Rascal…"). Não aplique se
  o relato disser quem foi ou o contexto indicar outro grupo. Regra do
  Eduardo; não conta como fato inventado.

```bash
# payload em $TMPDIR/pkd_mem_<slug>.json:
# {"title": "...", "content": "<p>...</p>", "date": {"year": 2026, "month": 9, "day": 23, "period": "almoco"}}
/opt/data/bin/pkd_memory.py create $TMPDIR/pkd_mem_<slug>.json --dry-run
/opt/data/bin/pkd_memory.py create $TMPDIR/pkd_mem_<slug>.json
/opt/data/bin/pkd_memory.py ping
```

`201`: o script grava `memory_id`, `id`, título e data em
`/opt/data/pkd_memorias.jsonl` (a única forma de reencontrar a Memória
depois). Confirme: título, data por extenso, `memory_id` e link.

### 4.3 Corrija uma Memória

Pegue o `memory_id` no registro (`pkd_memory.py log`) ou na conversa. Envie
**só o que mudou**: `title`, `content` e/ou `date`. `date` substitui a data
**inteira** (reenvie todos os componentes conhecidos). `content` substitui o
corpo inteiro: leia antes (`get`) e reescreva o HTML completo.

```bash
/opt/data/bin/pkd_memory.py get MEM-2026-09-23T12-7QF3K9
echo '{"date": {"year": 2026, "month": 9, "day": 22, "period": "almoco"}}' \
  | /opt/data/bin/pkd_memory.py patch MEM-2026-09-23T12-7QF3K9 -
```

O `memory_id` **não muda** depois da correção. Respostas do PATCH: `200`
corrigida; `404` ID não existe; `409` criptografada, título já usado ou
conflito de versão; `403` Memória bloqueada. Explique ao Eduardo.

## 5. Crie um Documento

```bash
# payload em $TMPDIR/pkd_doc_<slug>.json: {"title": "...", "content": "<h2>...</h2><p>...</p>"}
/opt/data/bin/pkd_doc.py create $TMPDIR/pkd_doc_<slug>.json [--attach /caminho/arq.pdf] --dry-run
/opt/data/bin/pkd_doc.py create $TMPDIR/pkd_doc_<slug>.json [--attach /caminho/arq.pdf]
/opt/data/bin/pkd_doc.py ping     # acesso + token, sem criar nada (espera 400)
/opt/data/bin/pkd_doc.py log      # documentos criados (/opt/data/pkd_documentos.jsonl)
```

- O Documento é criado na raiz da árvore. O Eduardo organiza depois.
- Documentos via Hermes são pequenos e de formatação simples. Para um texto
  muito longo, pergunte antes.

## Limites

- Pela API não há apagar, buscar nem listar. Documentos e Notas: só criar;
  editar é pela interface. Memórias: criar, ler e corrigir (§4.3).
- Memória sem `memory_id` no registro nem na conversa não pode ser corrigida
  pelo Hermes: peça o `memory_id` ao Eduardo ou que ele corrija na interface.

## Pitfalls

- **Cloudflare Access:** `pkd.dalc.in` fica atrás do Cloudflare Access. Cada
  caminho de API precisa de uma aplicação Access com política Bypass
  (skill `cloudflare-access`, `references/bypass-caminho.md`):
  `api/memories`, `api/notes` e `api/documents`. `302` para
  `edalcin.cloudflareaccess.com` = o bypass daquele caminho falta ou foi
  alterado: avise o Eduardo.
- `pkd_doc.py` tem um fallback de transição para o antigo `/api/import`
  (usado quando `/api/documents` ainda não aceita o token). Depois que o PKD
  novo estiver em produção, o fallback não é mais usado.
- **Nunca teste com `{}` em `pkd_doc.py`.** `ping` manda JSON inválido de
  propósito. (`pkd_note.py ping` e `pkd_memory.py ping` usam `{}`, que dá
  `400 title is required` sem criar nada.)
- O Cloudflare barra o User-Agent padrão do Python-urllib (`403 error code:
  1010`); os scripts enviam UA próprio. Em curl manual, defina `-A`.
- `GET /api/memories` e `GET /api/notes` (listas) só funcionam com a sessão
  da UI: com token dá `401`. Rotas por `memory_id` não aceitam o `id`
  numérico (`400 invalid memory id`).
- Não conclua capacidades da API por `OPTIONS`: confira no código em
  `/Storage/git/pkd`.
- `pkd2.dalc.in` era o ambiente de testes; não use.
~~~~
