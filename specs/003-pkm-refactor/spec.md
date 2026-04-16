# Feature Specification: PKM Refactor — Personal Knowledge Management System

**Feature**: `003-pkm-refactor`
**Created**: 2026-04-16
**Status**: Draft
**Input**: Refatoração completa do PKD em um sistema PKM com links bidirecionais, visualização em grafo, captura de conteúdo externo, documentação C4 Model, e interface moderna e criativa.

---

## Visão

PKD evolui de uma ferramenta simples de notas para um **Personal Knowledge Management (PKM) system** — um "segundo cérebro" que ajuda o usuário a armazenar, conectar e recuperar conhecimento. Os três pilares que guiam todo o design são:

1. **Curadoria** — Filtrar o que é importante, dando contexto a cada informação
2. **Conexão** — Links bidirecionais e tags que revelam como ideias se relacionam
3. **Recuperação** — Busca poderosa e navegação por conexões para encontrar qualquer coisa quando necessário

---

## User Scenarios & Testing

### User Story 1 — Criar e editar documentos ricos (Priority: P1)

O usuário cria documentos com título, corpo rico (formatação, imagens, tabelas, código) e um cabeçalho de metadados (tags, ícone, tipo). Documentos são organizados em hierarquia de pastas (documentos dentro de documentos). O editor usa TipTap com auto-save.

**Why this priority**: Sem a capacidade de criar e editar conteúdo rico, nenhuma outra funcionalidade tem valor. É o alicerce do "segundo cérebro".

**Independent Test**: Criar um documento raiz, adicionar filhos aninhados 3 níveis, formatar texto com vários blocos, colar imagem, redimensionar, salvar, recarregar — tudo deve persistir.

**Acceptance Scenarios**:

1. **Given** o sistema está rodando e o usuário autenticado, **When** clica em "Novo Documento", **Then** um documento com título editável, cabeçalho de metadados e editor TipTap aparece.
2. **Given** um documento existente, **When** o usuário edita título/corpo e para de digitar por 2 segundos, **Then** o conteúdo é salvo automaticamente e a versão incrementada.
3. **Given** um documento com filhos, **When** o usuário arrasta um documento para outra pasta na árvore, **Then** a hierarquia é atualizada sem perda de conteúdo.
4. **Given** o usuário cola uma imagem no editor, **When** o upload completa, **Then** a imagem aparece inline e pode ser redimensionada por alças de arrastar.

---

### User Story 2 — Conectar documentos com links bidirecionais (Priority: P1)

O usuário vincula documentos entre si. Ao criar um link de A para B, o documento B automaticamente mostra que A o referencia (backlink). Essas conexões formam uma rede de conhecimento navegável.

**Why this priority**: A conexão entre ideias é o "pilar mais crítico" do PKM. Sem links bidirecionais, o sistema é apenas uma pasta de arquivos.

**Independent Test**: Criar 3 documentos (A, B, C). Criar link de A→B e A→C. Verificar que B e C mostram A como backlink. Remover o link A→B. Verificar que B não mostra mais o backlink.

**Acceptance Scenarios**:

1. **Given** o documento A está aberto no editor, **When** o usuário digita `[[` e começa a digitar um nome, **Then** aparece uma lista de documentos filtrada para autocompletar.
2. **Given** o documento A tem um link para B, **When** o usuário abre B, **Then** B mostra uma seção "Referenciado por" com link de volta para A.
3. **Given** um link de A para B existe, **When** o usuário deleta o link em A, **Then** o backlink em B é automaticamente removido.
4. **Given** o documento B é excluído (trash), **When** o usuário abre A, **Then** o link para B é marcado como "documento excluído" (link quebrado visual).

---

### User Story 3 — Buscar e filtrar o conhecimento (Priority: P1)

Uma busca universal ("super-busca") permite encontrar qualquer documento por substring de título, corpo ou tags. Filtros por hashtag na árvore lateral mostram apenas documentos relevantes.

**Why this priority**: Recuperação é o terceiro pilar do PKM. Sem busca poderosa, o "segundo cérebro" se torna um "depósito esquecido".

**Independent Test**: Criar 10 documentos com texto e tags distintas. Buscar por substring de corpo, título e tag. Cada busca retorna apenas os documentos corretos, com snippet de contexto.

**Acceptance Scenarios**:

1. **Given** documentos existentes, **When** o usuário digita no campo de busca, **Then** resultados aparecem em tempo real (debounce 150ms) com snippets destacando o match.
2. **Given** tags existem, **When** o usuário seleciona uma ou mais tags no filtro, **Then** a árvore mostra apenas documentos que possuem TODAS as tags selecionadas (AND).
3. **Given** a busca retorna resultados, **When** o usuário clica em um resultado, **Then** o documento abre no editor com o trecho encontrado visível.

---

### User Story 4 — Visualizar conexões em grafo (Priority: P2)

O usuário abre uma "Graph View" que mostra os documentos como nós e os links bidirecionais como arestas. O grafo é interativo: zoom, pan, clique para abrir documento. Tags podem colorir os nós.

**Why this priority**: Visualizar a rede de conhecimento revela padrões e clusters de ideias que seriam invisíveis na árvore hierárquica. Essencial para o pilar de Conexão, mas requer que links (US2) já existam.

**Independent Test**: Criar 5 documentos com links entre eles. Abrir Graph View. Verificar que todos os nós e arestas aparecem. Clicar em um nó abre o documento. Zoom e pan funcionam.

**Acceptance Scenarios**:

1. **Given** documentos com links existem, **When** o usuário abre a Graph View, **Then** um grafo force-directed aparece com nós (documentos) e arestas (links).
2. **Given** o grafo está visível, **When** o usuário clica em um nó, **Then** o documento correspondente abre no editor.
3. **Given** o grafo está visível, **When** o usuário aplica filtro de tags, **Then** apenas documentos com as tags selecionadas são mostrados (nós restantes ficam esmaecidos).
4. **Given** um documento tem tags, **When** o grafo é renderizado, **Then** o nó correspondente é colorido de acordo com sua tag primária.

---

### User Story 5 — Capturar conteúdo externo (Priority: P2)

O usuário pode enviar conteúdo de outros aplicativos para o PKD: links da web, textos copiados, imagens, arquivos. No mobile, usa o menu "Compartilhar" do sistema operacional. No desktop, usa um endpoint API ou bookmarklet.

**Why this priority**: O pilar de Curadoria depende de capturar informação do mundo exterior rapidamente. Sem capture, o "segundo cérebro" depende de digitação manual.

**Independent Test**: Usar o mobile "Share" para enviar um link ao PKD. Verificar que um novo documento é criado com o título da página, URL no corpo, e tag `#captura`. Fazer o mesmo via API POST.

**Acceptance Scenarios**:

1. **Given** o usuário está em outro app no mobile, **When** seleciona "Compartilhar" e escolhe PKD, **Then** um novo documento é criado com o conteúdo compartilhado.
2. **Given** um endpoint de captura autenticado, **When** um POST é feito com `{title, content, tags, url}`, **Then** um documento é criado e retornado com status 201.
3. **Given** conteúdo capturado tem uma URL, **When** o documento é criado, **Then** os metadados da URL (Open Graph: título, descrição, imagem) são extraídos e armazenados.
4. **Given** o PWA está instalado, **When** o dispositivo registra o PKD como "share target", **Then** o menu de compartilhamento do SO lista o PKD.

---

### User Story 6 — Compartilhar documentos via link público (Priority: P2)

O usuário gera um link público para um documento específico. Qualquer pessoa com o link pode visualizar o documento em modo somente-leitura, sem navegação, com uma CSP restrita.

**Why this priority**: Permite compartilhar conhecimento selecionado sem expor todo o sistema.

**Independent Test**: Gerar link para um documento. Abrir em janela privada (sem sessão). O conteúdo renderiza read-only. Revogar o link. O URL retorna 404.

**Acceptance Scenarios**:

1. **Given** um documento, **When** o usuário clica "Compartilhar", **Then** um link único é gerado e copiado para o clipboard.
2. **Given** um link público válido, **When** qualquer pessoa o acessa, **Then** vê o documento renderizado em modo leitura.
3. **Given** o link foi revogado, **When** alguém acessa, **Then** recebe 404 (não 401, para não vazar existência).

---

### User Story 7 — Navegar por calendário (Priority: P3)

Um calendário mostra documentos organizados pela data de criação. Clicar em um dia lista os documentos criados naquele dia.

**Why this priority**: Útil para recuperação temporal, mas não essencial para o MVP.

**Independent Test**: Criar documentos em datas diferentes. Abrir calendário. Verificar que cada dia mostra os documentos corretos.

---

### User Story 8 — Administração e manutenção (Priority: P3)

Painel de administração para: backup/restore manual, limpeza de anexos órfãos, renomear/mesclar tags, esvaziar lixeira. Todas as operações de "higiene" do sistema.

**Why this priority**: Essencial para uso a longo prazo, mas não bloqueia o uso diário.

**Independent Test**: Fazer backup, alterar dados, restaurar, confirmar que o estado reverteu. Renomear tag usada por N documentos, verificar que todos atualizaram.

---

### User Story 9 — Temas, mobile e PWA (Priority: P3)

Interface com alternância claro/escuro, layout responsivo (mobile-first), instalação como PWA com modo offline read-only.

**Why this priority**: Qualidade de vida — a aplicação é funcional sem isso, mas o uso diário em celular exige.

**Independent Test**: Alternar tema, recarregar, tema persiste. Abrir no celular, toda interface usável com touch. Instalar PWA, desligar wifi, documentos recentes acessíveis read-only.

---

### Edge Cases

- **Link para documento excluído**: Links apontando para documentos no trash são marcados como "quebrados" visualmente, não removidos.
- **Ciclo de links**: A→B→C→A é válido — são links de referência, não hierarquia. O grafo renderiza ciclos normalmente.
- **Autorreferência**: Um documento pode referenciar a si mesmo (nota de "ver também" para diferentes seções).
- **Tag com zero documentos**: Após mover/deletar documentos, tags vazias permanecem até limpeza manual via admin.
- **Upload de imagem muito grande**: Retorna erro 413 com mensagem clara, sem salvar parcialmente.
- **Conflito de versão**: Duas abas editando o mesmo documento — mostrar diálogo overwrite/reload.
- **Service worker stale**: Cache do SW versionado; nova versão do app invalida cache anterior.
- **Share target failure**: Se o PWA recebe share mas está offline, enfileira para envio posterior (ou informa erro).

---

## Requirements

### Functional Requirements

#### Documentos & Hierarquia
- **FR-001**: O sistema DEVE permitir criar, editar, renomear e excluir documentos.
- **FR-002**: Cada documento DEVE ter: título, corpo rico (HTML via editor TipTap), metadados configuráveis (ícone, tags). Campo "tipo" foi removido — tags e ícone fornecem categorização suficiente (decisão Q7).
- **FR-003**: Documentos DEVEM ser organizáveis em hierarquia ilimitada (documentos dentro de documentos).
- **FR-004**: Documentos excluídos DEVEM ir para uma Lixeira e permanecer recuperáveis indefinidamente até exclusão permanente manual.
- **FR-005**: O sistema DEVE prevenir movimentos circulares na hierarquia (mover documento para dentro de um descendente seu).
- **FR-006**: Cada save DEVE incrementar uma versão monotônica. Saves com versão desatualizada DEVEM ser rejeitados com possibilidade de overwrite ou reload.

#### Links Bidirecionais
- **FR-010**: O sistema DEVE suportar links simples entre documentos (A→B), sem rótulo ou tipo — apenas source e target. O contexto do link é dado pelo texto ao redor da sintaxe `[[nome do documento]]` no corpo.
- **FR-011**: Ao criar um link de A para B, o sistema DEVE automaticamente registrar um backlink em B apontando para A.
- **FR-012**: Ao excluir um link de A para B, o backlink correspondente em B DEVE ser removido automaticamente.
- **FR-013**: Cada documento DEVE exibir uma seção "Referenciado por" listando todos os backlinks.
- **FR-014**: No editor, o usuário DEVE poder inserir links para outros documentos via autocompleção (trigger: `[[`).
- **FR-015**: Links para documentos excluídos DEVEM ser marcados visualmente como "quebrados" sem serem removidos.

#### Graph View
- **FR-020**: O sistema DEVE oferecer uma visualização em grafo (Graph View). Por padrão, o grafo mostra apenas documentos com ≥1 link (documentos isolados ficam ocultos). O usuário pode optar por exibir todos os documentos via toggle.
- **FR-021**: O grafo DEVE ser interativo: zoom, pan, clicar em nó abre o documento.
- **FR-022**: O grafo DEVE suportar filtro por tags (colorização de nós e/ou ocultação de nós sem match).
- **FR-023**: O grafo DEVE escalar para pelo menos 500 documentos com 2.000 conexões sem degradação perceptível.

#### Tags & Busca
- **FR-030**: Documentos DEVEM ser etiquetáveis com múltiplas hashtags (normalização: lowercase, sem `#`, sem espaços).
- **FR-031**: Uma busca universal DEVE encontrar documentos por substring de título, corpo ou tag.
- **FR-032**: Resultados de busca DEVEM incluir snippet com a parte correspondente destacada.
- **FR-033**: A árvore lateral DEVE ser filtrável por uma ou mais tags (semântica AND).
- **FR-034**: O administrador DEVE poder renomear ou mesclar tags (merge: quando o novo nome já existe).

#### Captura de Conteúdo Externo
- **FR-040**: O sistema DEVE expor um endpoint autenticado para captura de conteúdo externo via POST.
- **FR-041**: O PWA DEVE registrar-se como "share target" do SO para receber conteúdo compartilhado de outros apps.
- **FR-042**: Conteúdo capturado com URL DEVE tentar extrair metadados Open Graph (título, descrição, imagem preview).
- **FR-043**: Capturas DEVEM criar um novo documento com tag `#captura` por padrão (configurável).

#### Compartilhamento Público
- **FR-050**: O sistema DEVE gerar links públicos por documento (token único, revogável).
- **FR-051**: A view pública DEVE renderizar o documento em modo somente-leitura com política de segurança restrita (sem scripts).
- **FR-052**: Links revogados ou inexistentes DEVEM retornar 404 (nunca 401).

#### Calendário
- **FR-060**: O sistema DEVE oferecer uma view de calendário mostrando documentos por data de criação.
- **FR-061**: Clicar em um dia no calendário DEVE listar os documentos criados naquele dia.

#### Administração
- **FR-070**: O sistema DEVE oferecer backup manual por download (snapshot consistente do banco).
- **FR-071**: O sistema DEVE oferecer restore por upload de arquivo de backup com confirmação explícita.
- **FR-072**: O sistema DEVE oferecer limpeza de arquivos de anexo órfãos e compactação do banco.
- **FR-073**: A lixeira DEVE listar documentos excluídos com opções de restaurar ou excluir permanentemente (individual e em lote).

#### Autenticação & Segurança
- **FR-080**: A aplicação DEVE ser protegida por senha mestra fornecida via variável de ambiente.
- **FR-081**: Após 5 tentativas de login incorretas do mesmo IP, o sistema DEVE bloquear por 30 minutos.
- **FR-082**: Sessões DEVEM expirar após período configurável de inatividade (padrão: 60 min).
- **FR-083**: O sistema DEVE aplicar CSP, CSRF (double-submit cookie), HSTS, X-Frame-Options: DENY em todas as respostas.
- **FR-084**: Todo HTML de entrada DEVE ser sanitizado antes de armazenamento e antes de renderização pública.

#### Interface & PWA
- **FR-090**: A interface DEVE ter tema claro e escuro, persistido entre sessões.
- **FR-091**: A interface DEVE ser 100% funcional em dispositivos móveis (alvos de toque ≥ 44px).
- **FR-092**: A aplicação DEVE ser instalável como PWA com modo offline somente-leitura.
- **FR-093**: A interface DEVE incluir o logo/identidade visual do PKD.

#### Documentação C4 Model
- **FR-100**: O repositório DEVE conter documentação C4 Model nos quatro níveis: Context, Container, Component, Code.
- **FR-101**: Diagramas DEVEM ser escritos em Mermaid (renderizáveis no GitHub).
- **FR-102**: A documentação DEVE servir como referência arquitetural para implementação e evolução.

#### Implantação
- **FR-110**: A aplicação DEVE ser entregue como imagem Docker publicada em `ghcr.io/edalcin/pkd`.
- **FR-111**: Dados persistentes (banco de dados e anexos) DEVEM ficar em volumes externos ao container.
- **FR-112**: O documento `UNRAID.md` DEVE conter instruções completas para instalação via interface gráfica do UNRAID.
- **FR-113**: O repositório DEVE ter workflow CI/CD que gera nova imagem em cada push para `main`.
- **FR-114**: A imagem DEVE priorizar tamanho mínimo (meta: ≤ 30 MB).

### Key Entities

- **Document**: Título, corpo HTML, ícone, posição na hierarquia, versão, timestamps. Pode ter parent (hierarquia) e links (grafo). Sem campo "tipo" — categorização via tags.
- **Link**: Relação direcional simples entre dois documentos (source_id → target_id). Sem rótulo ou tipo. Unicidade: apenas um link de A→B pode existir (UNIQUE source, target). Backlinks são derivados automaticamente por consulta reversa.
- **Tag**: Nome normalizado (lowercase, sem `#`). Associação N:N com Document.
- **Attachment**: Arquivo no volume externo. Metadados: nome original, MIME, tamanho. Pertence a um Document.
- **ShareLink**: Token hasheado (SHA-256). Associação 1:1 com Document. Revogável.
- **Session**: In-memory, não persistida. Token + IP + timestamp de last-seen.
- **Capture**: Conteúdo recebido via API/share-target, convertido em Document com tag `#captura`.

---

## Success Criteria

### Measurable Outcomes

- **SC-001**: O usuário pode criar, editar e salvar um documento rico em menos de 5 segundos.
- **SC-002**: Busca por substring em 5.000+ documentos retorna resultados em menos de 1 segundo.
- **SC-003**: O grafo renderiza 500 nós com 2.000 arestas em menos de 3 segundos.
- **SC-004**: Backup de uma base com 10.000 documentos completa em menos de 10 segundos.
- **SC-005**: Renomear uma tag presente em 1.000 documentos completa em menos de 2 segundos.
- **SC-006**: A imagem Docker final tem ≤ 30 MB.
- **SC-007**: A interface é utilizável (todas funções acessíveis) em telas de 320px de largura.
- **SC-008**: Um usuário novo pode instalar o PKD no UNRAID via GUI sem abrir terminal.
- **SC-009**: Documentação C4 Model com 4 níveis (Context, Container, Component, Code) está presente e consistente.
- **SC-010**: Conteúdo compartilhado via mobile "Share" cria documento em menos de 3 segundos.
- **SC-011**: Links bidirecionais são atualizados em menos de 100ms após criação/remoção.

---

## Assumptions

- A aplicação é **single-user** (uma senha mestra, sem multi-tenancy).
- A refatoração é **parcial**: o backend Go (API, auth, store, segurança) é preservado e evoluído; o frontend é reescrito com **Svelte** para suportar graph view, links bidirecionais e uma UI mais rica (decisões Q4+Q5).
- O editor de documentos é **TipTap v2** (decisão já tomada na spec 002).
- O banco de dados é **SQLite** — arquivo local, sem dependências externas (decisão Q1).
- O Graph View usa **D3.js force-directed** renderizado no cliente (decisão Q2).
- A captura de conteúdo externo funciona **apenas online** — sem fila offline (decisão Q3).
- Backups são **manuais** — não há agendamento automático.
- A Lixeira retém documentos **indefinidamente** até exclusão manual.
- O modo offline do PWA é **somente leitura** — não há fila de escrita offline.
- Os diagramas C4 Model são escritos em **Mermaid** para renderização nativa no GitHub.
- Metadados Open Graph de URLs capturadas são extraídos **best-effort** — falha silenciosa se indisponível.
- A tag default de captura (`#captura`) é fixa na v1; configuração de tag pode vir em versão futura.

### Out of Scope

- Edição colaborativa em tempo real (multi-cursor / CRDT)
- Múltiplos usuários com permissões diferenciadas
- Integração com AI/LLM para sugestões de conexões
- Exportação para PDF ou outros formatos
- Versionamento de documentos com diff (apenas versão monotônica para conflito)
- Sincronização com serviços de nuvem (Google Drive, Dropbox)

---

## Clarifications

### Session 2026-04-16

- Q1: Banco de dados → **A: SQLite como único backend.** Arquivo local, zero config, imagem Docker menor (~20 MB). Sem dependência de serviço externo. Mantém o princípio de simplicidade e tamanho mínimo do Docker.
- Q2: Graph View → **A: Cliente interativo com D3.js force-directed.** Zoom, pan, clique para navegar, animação suave. Bundle extra ~100 KB. Mais interativo e útil para explorar conexões.
- Q3: Captura offline → **A: Apenas online.** Share target só funciona com conexão ativa. Se offline, mostra mensagem de erro amigável. Mais simples — evita fila de sincronização e IndexedDB.
- Q4: Escopo da refatoração → **B: Reescrita parcial.** Manter o backend Go (auth, API, store, segurança) e reescrever o frontend com um framework moderno para acomodar o graph view, links bidirecionais e uma UI mais rica.
- Q5: Framework do frontend → **C: Svelte.** Menor bundle (~15 KB gzipped, compila para vanilla JS), sem virtual DOM, reativo por padrão. Ideal para o princípio de simplicidade e tamanho mínimo do Docker. Integra com TipTap via svelte-tiptap e D3.js opera diretamente no DOM sem conflito.
- Q6: Modelo de links → **A: Link simples (source_id → target_id).** Sem rótulo, sem tipo. O contexto do link vem do texto ao redor do `[[link]]` no documento. Modelo Obsidian/Logseq — simplicidade máxima.
- Q7: Campo "tipo" do documento → **C: Remover.** Ícone + tags já fornecem categorização suficiente. O campo "tipo" seria redundante e adicionaria complexidade desnecessária ao modelo de dados e à UI.
- Q8: Graph View escopo inicial → **B: Apenas documentos conectados.** O grafo mostra por padrão apenas documentos que têm ≥1 link. Documentos isolados ficam ocultos. Limpo, performante, e foca no pilar de Conexão.
