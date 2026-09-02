# ADR-006: Chat RAG sobre os Documentos

**Status:** Aceito
**Data:** 2026-09-02
**Relacionado:** [ADR-002](002-hybrid-search-rrf-fusion.md) (a recuperação é a
busca híbrida, reusada inteira), [ADR-004](004-migracao-gemini-embedding-2.md)
(`semanticBodyChars` e a lição da whitelist de modelos)

## Contexto

O PKD tem ~283 documentos indexados por FTS5 e por embeddings Gemini, e uma
busca híbrida que funde as duas pernas por RRF. O que não existia era uma forma
de **perguntar** ao corpus: a busca devolve documentos, não respostas.

Recursos já presentes que a feature reusa sem modificação estrutural:
`GEMINI_API_KEY` (`config.go:90-93`), o cliente HTTP do Gemini em
`semantic.go:300-350`, `LexicalDocIDs` + `SemanticSearchDocIDs` + `FuseRRF`,
`ListByIDsFiltered`, a tabela `settings` e o par
`handleAdminGetSettings`/`handleAdminSetSettings`.

Duas restrições do estado atual moldam tudo o que segue:

1. **Um vetor por documento, sobre 800 caracteres** (`semanticBodyChars`). ADR-004
   já registra isso como "a maior perda de qualidade semântica do sistema".
2. **A busca híbrida sempre devolve resultados.** O `LIKE` casa qualquer
   substring e `semanticQueryFloor` é 0.30. Não existe estado "nada encontrado".

## Decisões

### D1 — Recuperação é a busca híbrida existente, sem retriever novo

A pergunta do usuário entra como `q` na mesma pipeline da busca:
`LexicalDocIDs` + `SemanticSearchDocIDs` fundidos por `FuseRRF`, filtrados por
`ListByIDsFiltered`. Zero código de recuperação novo.

**Por quê:** o léxico (FTS5) acerta exatamente onde o vetor truncado falha —
nomes próprios, siglas, termos raros. A fusão RRF já sabe combinar sinais de
qualidade desigual, o que é a propriedade que D6 explora abaixo.

**Alternativa descartada:** só a perna semântica ("RAG clássico"). Perderia
nomes próprios, que é metade das perguntas reais sobre notas pessoais.

**Alternativa descartada:** enviar o corpus inteiro (viável numa janela de 1M
tokens com 283 documentos). ~$1 por pergunta e diluição da atenção do modelo,
para substituir uma recuperação que já existe e é testada.

### D2 — Granularidade: recupera documento, envia documento inteiro; sem chunking

Os embeddings decidem **quais** documentos entram; o prompt recebe o `body_text`
**completo** dos selecionados. Nenhuma tabela de chunks, nenhum re-embed por
trecho, nenhum segundo espaço vetorial a gerenciar.

No mesmo movimento, `semanticBodyChars` sobe de **800 para 20.000 caracteres**
(~5k tokens, contra o limite real de 8.192 tokens do `gemini-embedding-2`),
pagando a dívida que ADR-004 deixou explícita. Isso melhora a recuperação de
**todos** os consumidores — busca e Graph View —, não só do Chat, e exige um
re-embed completo do corpus (~$0,45).

Consequência: a atribuição é em nível de documento ("segundo *Fotossíntese*"),
nunca de parágrafo.

**Alternativa descartada:** tabela `document_chunks` com um vetor por trecho.
É a solução correta em precisão e custa uma reescrita de `semantic.go`, uma
migração, staleness por chunk e a convivência de dois níveis de vetor no
`GetSemanticEdges`. **Alternativa pré-decidida:** o chunking entra quando se
observar resposta errada *porque o documento certo não foi recuperado* — falha
de recuperação, não de geração. Sinal de alerta a monitorar: documentos longos e
temáticamente heterogêneos são o caso em que D2 quebra primeiro.

**Alternativa descartada:** truncar o documento que não cabe no orçamento
(D3). É chunking pela porta de trás com o pior dos dois mundos — o modelo cita
um documento cujo trecho relevante pode ter ficado fora.

### D3 — Orçamento de tokens com omissão, não N fixo

Percorre o ranking fundido somando o tamanho de cada documento, até no máximo 8
documentos ou o teto de tokens (~100k), o que vier primeiro. Documento que não
cabe é **omitido** (nunca truncado), e a UI sinaliza a omissão. A contagem é a
aproximação `len(body)/4`, sem tokenizer nem dependência nova.

**Por quê:** com D2 enviando corpos completos, o custo por pergunta é função do
tamanho dos documentos recuperados — que é dado que ninguém controla. O teto é a
única coisa entre uma pergunta e uma fatura surpresa.

### D4 — Grounding estrito com corte antes da chamada

O prompt de sistema ordena responder **apenas** com base nos documentos
fornecidos e dizer explicitamente quando não houver base. Além disso, a nova
constante `chatRelevanceFloor` (~0.50) exige que o melhor candidato semântico
atinja um piso — abaixo dele o backend responde "nada relevante encontrado"
**sem chamar o modelo**.

**Por quê:** D1 herda uma busca que nunca retorna vazio. "Qual a capital da
França" recupera oito documentos irrelevantes, e o modelo responde de cabeça com
uma lista de fontes que não sustenta nada — resposta que *parece* fundamentada.
É o pior modo de falha da feature, e o corte o elimina enquanto economiza a
chamada paga.

O piso é **separado** de `semanticQueryFloor = 0.30`: reusá-lo não filtraria
nada, e subi-lo degradaria a busca para consertar o Chat. Este é o primeiro
consumidor real de `store.SemanticHit.Score`, que ADR-004 deixou como
write-only justamente como ponto de extensão — nenhum código de scoring novo.

**Alternativa descartada:** modelo livre, usando os documentos como apoio.
Apaga a fronteira entre "está no meu PKD" e "o modelo acha que sim", que é a
fronteira da qual um PKM extrai seu valor.

### D5 — Atribuição servida pelo backend, não pelo modelo

A resposta carrega a lista de **Documentos Consultados** (IDs e títulos que
foram efetivamente ao prompt), renderizada como links `#/doc/{id}` que abrem em
**nova aba**. Citação inline no texto é conveniência opcional.

**Por quê:** a lista é dado que o servidor já tem em mãos, custa zero prompt
engineering e é impossível de alucinar. Um modelo que esquece de citar não pode
custar a rastreabilidade inteira.

### D6 — Multi-turno: query concatenada, não reescrita por modelo

`q` é a concatenação das ~3 últimas mensagens **do usuário**. "E sobre a fase
escura?" busca junto com "o que eu anotei sobre fotossíntese?", recuperando o
contexto que a pergunta isolada não carrega.

**Por quê:** duas linhas contra uma chamada de API inteira. Uma query
concatenada é ruim para embedding e **ótima** para FTS5, e a fusão RRF de
ADR-002 já é feita para combinar duas listas de qualidade desigual — a
arquitetura da busca híbrida paga dividendo aqui.

**Alternativa descartada:** reescrita de query pelo modelo (padrão da
indústria). Dobra as chamadas, dobra a latência **antes do primeiro token** —
exatamente o que D7 existe para esconder — e adiciona um ponto de falha.
**Alternativa pré-decidida** se conversas longas recuperarem mal: substituir a
montagem de `q` por uma chamada de reescrita, sem tocar em mais nada.

**Alternativa descartada:** recuperar só no primeiro turno. Torna impossível
mudar de assunto no meio da conversa.

### D7 — SSE via `POST`, com `Flusher` e cancelamento por contexto

`POST /api/chat` responde `text/event-stream`, consumido no frontend por `fetch`
+ `ReadableStream` + `AbortController`. Backend chama
`:streamGenerateContent?alt=sse`, faz `Flush()` a cada chunk e propaga
`r.Context()` à chamada do Gemini.

Este é o primeiro endpoint de streaming do PKD, e é a única decisão desta série
que escolhe deliberadamente **mais** código: ~30 linhas no handler, ~15 no
componente. Num chat, 20 segundos de tela parada não é lentidão, é "parece
quebrado" — e o usuário reenvia a pergunta, dobrando o custo.

**Alternativa descartada:** `GET` + `EventSource`. `EventSource` não permite
headers customizados, então exigiria **isentar a rota do CSRF global** — nunca
se enfraquece uma defesa para acomodar uma escolha de transporte. A pergunta e o
histórico também iriam na query string, com limite de tamanho e vazamento para
logs de acesso. Bônus: a reconexão automática que se perde é indesejável, pois
reenviaria a pergunta e pagaria a geração duas vezes.

**Alternativa descartada:** resposta única com spinner. Não gera retrabalho na
recuperação, mas o incremento é pequeno e o ganho de percepção é o maior desta
feature.

### D8 — Conversa efêmera + "salvar como documento"

O estado vive no componente da rota `#/chat`; trocar de rota ou recarregar
destrói a conversa. Um botão transforma a conversa num documento PKD normal
(título = primeira pergunta, corpo = conversa em HTML), via `DocumentStore`.

**Por quê:** o valor durável de uma resposta boa é ela virar conhecimento
indexado, versionado e linkável — o que o `DocumentStore` já faz. Um histórico
de conversas seria um segundo sistema de armazenamento de texto ao lado do que
já existe, para reler coisas que raramente se relê.

**Alternativa descartada:** tabelas `chat_conversations`/`chat_messages` com
histórico navegável. Duas tabelas, migração, endpoints de listar/abrir/apagar e
uma tela nova, por imitação de produto.

**Alternativa pré-decidida** se perder a conversa ao navegar incomodar: mover o
estado para uma store de módulo (`chat.js`), ~5 linhas, sobrevive à troca de
rota mas não ao reload. Os links de fonte abrindo em nova aba (D5) já evitam o
caso mais frequente.

### D9 — Whitelist compilada de dois modelos, com o preview rotulado

Chave `chat.model` na tabela `settings`, whitelist compilada no pacote `server`
e espelhada no `<select>` de Administração → Preferências:

| Identificador | Rótulo | Status na API |
|---|---|---|
| `models/gemini-3.7-flash` | Flash (padrão) | GA |
| `models/gemini-3.1-pro` | Pro (preview) | **preview** |

O default compilado é o Flash. Não existe modelo **Pro em GA** hoje
(`gemini-3.1-pro` é preview; a família 2.0 foi deprecada em 01/06/2026), e as
duas outras opções GA — `3.5-flash` e `3.6-flash` — são mais caras e não
melhores que a `3.7-flash`, ou seja, escolhas estritamente dominadas.

**Por quê oferecer um preview:** um dropdown com uma única opção não é uma
escolha. Flash vs Pro é a única distinção com significado real (rápido/barato vs
raciocínio mais forte). O risco é o de ADR-004 — modelo que muda de nome ou sai
do ar — e é **gerenciável aqui porque a falha é visível**: o Chat responde com
erro na cara do usuário, ao contrário da busca semântica, que degradava em
silêncio para o léxico. O rótulo "(preview)" no `<select>` é o que converte
risco oculto em risco informado.

**Alternativa descartada:** listar `GET /v1beta/models` em tempo real. Traz
~40 modelos irrelevantes (embedding, TTS, imagem, previews) e uma chamada de
rede que pode falhar no carregamento da tela de admin.

### D10 — Header `x-goog-api-key`; `embedBatch` migra junto

A chamada de chat autentica por header `x-goog-api-key`, hoje a forma
recomendada pela doc. `embedBatch` (`semantic.go:300-350`), que usa
`?key={apiKey}` na URL, migra no mesmo passo.

**Por quê:** query params vazam para logs de proxy e de acesso. São duas linhas
num caller já existente, e deixar dois padrões de autenticação convivendo no
mesmo pacote é convenção paralela.

### D11 — Sem rate limiting próprio; `429` é mensagem, não retry

O PKD é single-user autenticado; o limite efetivo é o do Gemini. Um `429`
`RESOURCE_EXHAUSTED` vira mensagem clara na UI, **sem retry automático**.

**Por quê:** retry automático numa chamada de geração paga o mesmo trabalho
duas vezes. O usuário decide se repete.

### D12 — `generationConfig` mínima

`maxOutputTokens: 4096`. `temperature` **não é enviada**: a doc do Gemini 3.x
avisa que alterar o default degrada raciocínio, e mandar o default explicitamente
é uma constante a manter sem efeito.

## Consequências

- `internal/store/semantic.go`: `semanticBodyChars` 800 → 20.000 (re-embed
  completo do corpus no primeiro sweep após o deploy); `embedBatch` passa a
  autenticar por header.
- Novo caminho de chat no `internal/server`: handler `POST /api/chat` com SSE,
  montagem do prompt, orçamento de tokens e o corte por `chatRelevanceFloor`.
- `internal/store/settings.go`: chave `chat.model` com método wrapper; nenhuma
  migração de schema (a tabela `settings` já existe).
- `internal/server/handlers_admin.go`: `case "chat.model"` em
  `handleAdminSetSettings` (hoje toda chave fora da whitelist é rejeitada com
  400) e a chave exposta em `handleAdminGetSettings`.
- `frontend`: nova rota `#/chat` em `App.svelte` (`getRoute` + bloco
  condicional), ícone na topbar **desabilitado com tooltip** quando
  `embed.key_configured` for `false`, componente `Chat.svelte` com leitura de
  stream, e o primeiro `<select>` de modelo da tela de Preferências.
- Documentos protegidos por senha ficam fora do Chat automaticamente:
  `EmbedStaleDocs` já exclui `encrypted`. O universo é o mesmo da busca —
  ativos + arquivados, sem lixeira. Anexos (ADR-005) não têm embedding e estão
  fora.
- Glossário: `Chat`, `Modelo de Chat`, `Modelo de Embedding`,
  `Documentos Consultados`, `Piso de Relevância do Chat`.

## Riscos aceitos, com o gatilho de reversão

| Risco | Gatilho para agir |
|---|---|
| Recuperação grosseira (D2, sem chunking) | Resposta errada porque o documento certo não foi recuperado → `document_chunks` |
| `chatRelevanceFloor = 0.50` é um chute | Perguntas legítimas recusadas, ou irrelevantes passando → ajustar a constante |
| `gemini-3.1-pro` é preview (D9) | Erro do modelo na UI → remover a opção numa release |
| Custo por pergunta cresce com o tamanho dos documentos (D2/D3) | Teto de tokens já limita; revisar o teto, não a política |
