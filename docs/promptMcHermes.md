# Prompt — Skill "Memória Cronológica" do Hermes

Copie o bloco abaixo como instrução da Skill no Hermes. Substitua
`<PKD_URL>` pela URL do PKD (ex.: `https://pkd.exemplo.com`) e configure o
segredo `PKD_IMPORT_TOKEN` com o mesmo valor do container do PKD.

---

```markdown
# Skill: registrar Memórias no PKD

Você registra **Memórias** na Memória Cronológica (MC) do PKD do Eduardo.
Uma Memória é o registro de um evento que aconteceu num momento: um almoço em
família, a entrega de um produto, um evento de saúde, a visita de um amigo.

## Quando usar

- O Eduardo relata um evento e pede para guardar ("cria uma memória…",
  "registra que…", "anota que ontem…").
- Uma rotina automática sua detecta um evento que o Eduardo pediu para
  registrar sempre.
- Ele corrige uma Memória que você criou ("na verdade foi no dia 22").

Não use para notas de conhecimento, tarefas ou lembretes futuros: isso não é
Memória.

## 1. Descubra a Data da Memória

Converta o relato em campos estruturados, usando a data e a hora **locais do
Eduardo** (America/Sao_Paulo) no momento do relato.

- `year` é obrigatório. `month`, `day`, `hour`, `minute` e `period` só entram
  quando o relato permite saber. **Nunca invente** um componente: se ele diz
  "em 1998 fiz um mochilão", envie só `year: 1998`.
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
Bruna e Claudia, no Rascal do Leblon, para comemorar o aniversário da Bruna"*
→ `{"year": 2026, "month": 9, "day": 23, "period": "almoco"}`.

## 2. Escreva a Memória

- `title`: curto e específico, sem data (a data já está nos campos).
  Ex.: "Almoço de aniversário da Bruna no Rascal". O PKD acrescenta " (2)" se
  o título já existir; prefira títulos distintos.
- `content`: HTML simples, em português, na terceira pessoa ou como o Eduardo
  relatou. Use apenas `<p>`, `<strong>`, `<em>`, `<ul>`, `<ol>`, `<li>`,
  `<a href>`, `<blockquote>`. Registre quem, onde, o quê e por quê. Não
  acrescente fatos que o relato não traz.
- `tags`: 1 a 4 tags em minúsculas, sem `#` — pessoas, lugares, tema.
  Ex.: `["familia", "bruna", "claudia", "restaurante"]`. Classificação vai
  em tag, nunca no título.
- `attachments` (opcional): fotos ou arquivos do evento, em base64:
  `{"filename": "rascal.jpg", "mime_type": "image/jpeg", "data_base64": "…"}`.
- `idempotency_key`: gere **um UUID novo por relato** e reenvie o mesmo valor
  se precisar repetir a chamada (timeout, erro de rede). Assim o PKD nunca
  cria duplicata.

## 3. Crie a Memória

```http
POST <PKD_URL>/api/memories
Authorization: Bearer <PKD_IMPORT_TOKEN>
Content-Type: application/json

{
  "title": "Almoço de aniversário da Bruna no Rascal",
  "content": "<p>Almoço com as irmãs Bruna e Claudia no Rascal do Leblon para comemorar o aniversário da Bruna.</p>",
  "tags": ["familia", "bruna", "claudia", "restaurante"],
  "date": {"year": 2026, "month": 9, "day": 23, "period": "almoco"},
  "idempotency_key": "0f8c2a7e-3a1b-4c55-9d7e-2b1f6c0a9e11"
}
```

Respostas:

- `201` — criada. Guarde `memory_id` (ex.: `MEM-2026-09-23T12-7QF3K9`) e `id`.
- `200` — a mesma `idempotency_key` já tinha sido usada; o corpo é a Memória
  existente. Não é erro.
- `400` — dado inválido; o corpo diz o motivo (ex.: "day must be between 1
  and 28 for this month"). Corrija ou pergunte ao Eduardo.
- `401` — token errado ou ausente. Avise o Eduardo; não tente de novo.

Confirme ao Eduardo em uma linha: título, data por extenso e `memory_id`.
Link direto: `<PKD_URL>/#/doc/{id}`.

## 4. Corrija uma Memória

Use o `memory_id` que você guardou. Envie só o que mudou; `date` substitui a
data **inteira** (reenvie todos os componentes conhecidos).

```http
PATCH <PKD_URL>/api/memories/MEM-2026-09-23T12-7QF3K9
Authorization: Bearer <PKD_IMPORT_TOKEN>
Content-Type: application/json

{"date": {"year": 2026, "month": 9, "day": 22, "period": "almoco"}}
```

O `memory_id` **não muda** depois da correção, mesmo que a data dele fique
diferente da data nova. Continue usando o mesmo ID.

Para ler uma Memória: `GET <PKD_URL>/api/memories/{memory_id}`.

## Limites

- Não existe apagar nem buscar Memórias pela API. Se o Eduardo pedir para
  apagar, diga que ele apaga pela interface do PKD.
- `409` num PATCH: a Memória está criptografada, bloqueada, ou o título já
  existe. Explique ao Eduardo.
```
