# Prompt — Skill "PKD" do Hermes

Uma Skill só para os três tipos de conteúdo do PKD: **Nota**, **Memória** e
**Documento** (termos em [`adr/glossary.md`](adr/glossary.md)). Ela substitui
as Skills antigas de Memórias e `pkd-documentos`: apague as duas no Hermes.

Copie o bloco abaixo como instrução da Skill no Hermes. Substitua
`<PKD_URL>` pela URL do PKD (ex.: `https://pkd.dalc.in`) e configure o
segredo `PKD_IMPORT_TOKEN` com o mesmo valor do container do PKD.

---

````markdown
# Skill: criar conteúdo no PKD

O PKD é a base de conhecimento pessoal do Eduardo. Você cria nele três tipos
de conteúdo: **Nota**, **Memória** e **Documento**.

## Quando usar

O Eduardo pede para guardar algo **no PKD**: "crie uma nota no PKD…",
"registra no PKD que…", "crie um documento no PKD sobre…", "guarda isso no
PKD". Ele também pode corrigir uma Memória que você criou ("na verdade foi no
dia 22").

Não use quando o pedido não cita o PKD (ex.: só "cria uma memória"). Isso é
um pedido para a sua própria memória do Hermes. Na dúvida, pergunte.

## 1. Escolha o tipo

| Tipo | O que é | Exemplos |
|---|---|---|
| **Nota** | Informação curta e rápida, sem data de evento | endereço, telefone, senha do Wi-Fi da casa da mãe, lista de compras, ideia solta |
| **Memória** | Registro de um **evento** que aconteceu num momento | almoço em família, consulta médica, entrega de um produto, visita de um amigo |
| **Documento** | Texto organizado e duradouro, com seções | resumo de uma pesquisa, roteiro de viagem, receita, procedimento passo a passo |

- Se o Eduardo diz o tipo ("crie uma **nota**", "uma **memória**", "um
  **documento**"), use esse tipo.
- Se ele não diz o tipo, escolha pela tabela. Se ainda houver dúvida entre
  dois tipos, pergunte em uma linha ("Nota ou Documento?").
- Tarefa ou lembrete futuro não é Memória. Uma Memória é sempre algo que já
  aconteceu.

## 2. Regras comuns aos três tipos

- `title`: curto e específico. Se o título já existir, o PKD acrescenta
  " (2)". Prefira títulos diferentes.
- `content`: HTML simples, em português. Use só estas tags: `<p>`, `<h2>`,
  `<h3>`, `<strong>`, `<em>`, `<ul>`, `<ol>`, `<li>`, `<a href>`,
  `<blockquote>`, `<pre><code>`, `<table>`, `<tr>`, `<th>`, `<td>`. Não use
  Markdown: ele aparece como texto cru. Não acrescente fatos que o Eduardo não
  disse.
- `tags` (opcional): lista de palavras, ex.: `["saude", "familia"]`. Só envie
  tags quando o Eduardo pedir ou quando ele já usou a tag antes.
- `attachments` (opcional): fotos ou arquivos, em base64:
  `{"filename": "foto.jpg", "mime_type": "image/jpeg", "data_base64": "…"}`.
- `idempotency_key`: gere **um UUID novo por pedido**. Se precisar repetir a
  chamada (timeout, erro de rede), reenvie o **mesmo** valor. Assim o PKD
  nunca cria duplicata.
- Cabeçalhos de toda chamada:

  ```http
  Authorization: Bearer <PKD_IMPORT_TOKEN>
  Content-Type: application/json
  ```

Respostas comuns:

- `201`: criado.
- `200`: a mesma `idempotency_key` já foi usada. O corpo é o item que já
  existe. Não é erro.
- `400`: dado inválido. O corpo diz o motivo. Corrija ou pergunte ao Eduardo.
- `401`: token errado ou ausente. Avise o Eduardo e não tente de novo.
- `413`: anexo grande demais. Avise o Eduardo.

## 3. Crie uma Nota

```http
POST <PKD_URL>/api/notes

{
  "title": "Endereço da Bruna",
  "content": "<p>Rua das Flores, 120, apto 302 — Leblon, Rio de Janeiro.</p>",
  "idempotency_key": "6b1d6c3e-8f0a-4c1e-9a55-0d6f7e2b4a10"
}
```

Confirme em uma linha: título e link `<PKD_URL>/#/doc/{id}` (o PKD abre a
Nota no Mural de Notas).

## 4. Crie uma Memória

### 4.1 Descubra a Data da Memória

Converta o relato em campos, com a data e a hora **locais do Eduardo**
(America/Sao_Paulo) no momento do relato.

- `year` é obrigatório. `month`, `day`, `hour`, `minute` e `period` só entram
  quando o relato permite saber. **Nunca invente** um componente: "em 1998
  fiz um mochilão" → só `year: 1998`.
- Resolva datas relativas: "ontem", "anteontem", "sábado passado", "semana
  passada na quinta".
- Um relato feito entre 00:00 e 05:59 com "ontem" ou "hoje" é ambíguo:
  pergunte qual dia antes de criar.
- Hora exata → `hour` (0–23) e, se souber, `minute` (0–59).
- Momento vago → `period`. Use **ou** `hour` **ou** `period`, nunca os dois:

  | Relato | `period` |
  |---|---|
  | de madrugada (00–06h) | `madrugada` |
  | de manhã (06–12h) | `manha` |
  | no almoço (12–15h) | `almoco` |
  | à tarde (12–18h) | `tarde` |
  | no lanche, no café da tarde (16–18h) | `lanche` |
  | no jantar (18–21h) | `jantar` |
  | à noite (18–24h) | `noite` |

- Hora ou período exigem `day`; `day` exige `month`.
- O título não leva data: a data já está nos campos.

Exemplo: relato em 24/09/2026 às 15h, *"ontem almocei com minhas irmãs no
Rascal do Leblon"* → `{"year": 2026, "month": 9, "day": 23, "period": "almoco"}`.

### 4.2 Envie

```http
POST <PKD_URL>/api/memories

{
  "title": "Almoço de aniversário da Bruna no Rascal",
  "content": "<p>Almoço com as irmãs Bruna e Claudia no Rascal do Leblon para comemorar o aniversário da Bruna.</p>",
  "date": {"year": 2026, "month": 9, "day": 23, "period": "almoco"},
  "idempotency_key": "0f8c2a7e-3a1b-4c55-9d7e-2b1f6c0a9e11"
}
```

Guarde o `memory_id` da resposta (ex.: `MEM-2026-09-23T12-7QF3K9`). Confirme
em uma linha: título, data por extenso, `memory_id` e link
`<PKD_URL>/#/doc/{id}`.

### 4.3 Corrija uma Memória

Use o `memory_id` que você guardou. Envie só o que mudou. `date` substitui a
data **inteira**: reenvie todos os componentes conhecidos.

```http
PATCH <PKD_URL>/api/memories/MEM-2026-09-23T12-7QF3K9

{"date": {"year": 2026, "month": 9, "day": 22, "period": "almoco"}}
```

O `memory_id` **não muda** depois da correção. Para ler uma Memória:
`GET <PKD_URL>/api/memories/{memory_id}`.

## 5. Crie um Documento

```http
POST <PKD_URL>/api/documents

{
  "title": "Roteiro de viagem — Lisboa",
  "content": "<h2>Dia 1</h2><ul><li>Alfama</li><li>Castelo de São Jorge</li></ul><h2>Dia 2</h2><p>Belém e Pastéis de Belém.</p>",
  "idempotency_key": "a3c9e2f1-5b7d-4e8a-b0c4-91d2f6e8a7b3"
}
```

- O Documento fica na raiz da árvore. O Eduardo organiza depois.
- Só envie `parent_id` (número) quando o Eduardo der o ID do Documento-pai.
  `400 parent not found` = o ID não existe.
- Documentos criados pela API são pequenos e de formatação simples. Para um
  texto muito longo, pergunte antes ao Eduardo.

Confirme em uma linha: título e link `<PKD_URL>/#/doc/{id}`.

## Limites

- Apagar, buscar ou listar conteúdo do PKD não é possível pela API. Se o
  Eduardo pedir, diga que ele faz isso pela interface do PKD.
- Documentos: só criar. Para editar, o Eduardo usa a interface.
- `409` num PATCH de Memória: ela está criptografada, o título já existe ou
  houve conflito de versão. `403`: a Memória está bloqueada. Explique ao
  Eduardo.
````
