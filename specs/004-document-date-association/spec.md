# Feature Specification: Document Date Association

**Feature Branch**: `004-document-date-association`  
**Created**: 2026-04-25  
**Status**: Draft  
**Input**: User description: "Quero adicionar uma nova funcionalidade. Na área de 'Associações' quero poder associar uma data, separadamente em dia, mês e ano. Quando criar um novo documento, o dia, mês e ano já vem preenchido com o dia atual. Porém, vou poder alterar. Pode haver um documento com apenas o ano associado, ou apenas o mês e ano associados. Nunca somente o dia ou somente o mês. Os documentos já existentes terão associados a sua data de criação. A data de criação sempre haverá associada nativamente ao documento e será fiel a data em que o documento foi criado e nunca poderá ser editada. A 'data associada' será uma segunda data, atribuída pelo usuário, que poderá ser editada. Quero selecionar o ano, o mês e o dia para associar por meio de um pulldown ou algo parecido, com uma boa experiência para o usuário."

---

## Clarifications

### Session 2026-04-25

- Q: O seletor de ano deve permitir anos futuros além do ano atual? → A: Sim, até ano atual + 10 anos futuros.
- Q: Como os campos não preenchidos aparecem no formulário de edição de data parcial? → A: Todos os três campos sempre visíveis; campos não preenchidos aparecem vazios (sem seleção).
- Q: Como o usuário remove completamente a data associada? → A: Há um botão "Limpar data" que zera todos os campos de uma vez.
- Q: Ao remover o mês de uma data completa, o campo de dia deve ser automaticamente limpo? → A: Sim, o dia é automaticamente limpo em cascata quando o mês é removido.
- Q: Como datas parciais são ordenadas em listas? → A: Como início do período — ano-only equivale a 1º de janeiro; mês+ano equivale ao 1º dia do mês.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Associar data a um novo documento (Priority: P1)

Ao criar um novo documento, o usuário deseja registrar uma data associada que reflita quando o evento ou conteúdo ocorreu — que pode ser diferente da data de criação do documento no sistema. Os campos de dia, mês e ano são apresentados separadamente e já vêm preenchidos com a data atual, mas podem ser alterados livremente. O usuário pode optar por registrar apenas o ano, apenas o mês e o ano, ou a data completa.

**Why this priority**: É o núcleo da funcionalidade. Sem isso, nenhum outro cenário faz sentido.

**Independent Test**: Pode ser testado criando um novo documento, verificando que os campos de data já aparecem preenchidos com a data atual, alterando os valores e salvando — então confirmando que a data associada foi salva corretamente.

**Acceptance Scenarios**:

1. **Given** o usuário está criando um novo documento, **When** a área de Associações é exibida, **Then** os campos de dia, mês e ano mostram a data atual como valor padrão.
2. **Given** o usuário limpa os campos de dia e mês, deixando apenas o ano, **When** salva o documento, **Then** a data associada é salva somente com o ano.
3. **Given** o usuário limpa apenas o campo de dia, deixando mês e ano, **When** salva o documento, **Then** a data associada é salva com mês e ano.
4. **Given** o usuário preenche todos os três campos, **When** salva o documento, **Then** a data associada é salva com a data completa (dia, mês e ano).
5. **Given** o usuário tenta salvar com apenas o dia preenchido (sem mês ou ano), **When** tenta salvar, **Then** o sistema exibe uma mensagem de validação impedindo o salvamento.
6. **Given** o usuário tenta salvar com apenas o mês preenchido (sem ano), **When** tenta salvar, **Then** o sistema exibe uma mensagem de validação impedindo o salvamento.
7. **Given** o usuário tenta salvar com dia e ano mas sem mês, **When** tenta salvar, **Then** o sistema exibe uma mensagem de validação impedindo o salvamento.

---

### User Story 2 - Editar a data associada de um documento existente (Priority: P2)

O usuário deseja alterar a data associada de um documento já criado. A data de criação do sistema (imutável) continua visível para referência, mas a data associada pode ser editada para refletir uma data diferente.

**Why this priority**: Dados podem precisar de correção. A edição é essencial para manter a informação útil ao longo do tempo.

**Independent Test**: Pode ser testado abrindo um documento existente, alterando a data associada na área de Associações e verificando que a nova data foi salva enquanto a data de criação permanece inalterada.

**Acceptance Scenarios**:

1. **Given** o usuário abre um documento existente, **When** visualiza a área de Associações, **Then** a data associada atual é exibida nos campos correspondentes.
2. **Given** o usuário altera a data associada e salva, **When** reabre o documento, **Then** a nova data associada é exibida corretamente.
3. **Given** o usuário visualiza a área de Associações, **When** observa a data de criação do documento, **Then** não há nenhum controle de edição disponível para ela — ela é somente leitura.
4. **Given** o usuário remove todos os valores da data associada, **When** salva, **Then** o documento fica sem data associada (campo vazio é permitido).

---

### User Story 3 - Migração de documentos existentes (Priority: P3)

Documentos já existentes no sistema devem receber automaticamente, como data associada inicial, a mesma data de sua criação. O usuário deve poder alterar essa data associada posteriormente, mas ela já estará preenchida ao abrir o documento pela primeira vez após a migração.

**Why this priority**: Garante que o sistema não quebre dados históricos e que o usuário não precise repreencher datas em todos os documentos.

**Independent Test**: Pode ser testado verificando que um documento criado antes da implantação da funcionalidade já exibe, na área de Associações, a sua data de criação como data associada padrão.

**Acceptance Scenarios**:

1. **Given** um documento foi criado antes da implantação desta funcionalidade, **When** o usuário abre a área de Associações, **Then** a data associada exibe a data de criação do documento.
2. **Given** um documento migrado com data associada igual à criação, **When** o usuário altera e salva a data associada, **Then** a data de criação permanece inalterada e apenas a data associada muda.

---

### Edge Cases

- O que acontece quando o usuário tenta selecionar um dia inválido para um mês (ex.: 31 de fevereiro)? O seletor deve oferecer apenas dias válidos para o mês escolhido.
- O que acontece quando o usuário seleciona um ano sem mês — o campo de dia deve permanecer desabilitado?
- Como o sistema exibe a data associada quando apenas o ano está preenchido (ex.: "2024")?
- Como o sistema exibe a data quando há apenas mês e ano (ex.: "Abril/2024")?
- O que acontece se o usuário remover apenas o dia de uma data completa — mês e ano são mantidos? **Resolvido**: sim, mês e ano são mantidos; apenas remover o mês dispara cascata no dia.
- O que acontece quando o usuário remove o mês de uma data completa — o dia é automaticamente limpo? **Resolvido**: sim, cascata automática (FR-011a).
- Como o seletor de anos trata anos bissextos ao exibir dias de fevereiro?

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: O sistema DEVE exibir, na área de Associações de cada documento, três campos separados para dia, mês e ano da "data associada".
- **FR-002**: O sistema DEVE preencher automaticamente os campos de dia, mês e ano com a data atual ao criar um novo documento.
- **FR-003**: O usuário DEVE poder selecionar o ano, mês e dia por meio de controles de seleção (pulldown ou equivalente) com boa usabilidade.
- **FR-003a**: O seletor de ano DEVE oferecer o intervalo de 1900 até o ano atual acrescido de 10 anos futuros.
- **FR-004**: O sistema DEVE aceitar e salvar as seguintes combinações de data associada: somente ano; mês e ano; dia, mês e ano.
- **FR-005**: O sistema DEVE rejeitar e exibir mensagem de validação ao tentar salvar combinações inválidas: somente dia; somente mês; dia com ano mas sem mês.
- **FR-006**: O sistema DEVE armazenar a data de criação do documento de forma imutável, fiel ao momento em que o documento foi criado.
- **FR-007**: O sistema DEVE exibir a data de criação do documento como informação somente leitura, sem possibilidade de edição pelo usuário.
- **FR-008**: O sistema DEVE permitir que a data associada seja editada a qualquer momento após a criação do documento.
- **FR-009**: O sistema DEVE pré-preencher a data associada de documentos existentes com a respectiva data de criação, como parte da migração na implantação.
- **FR-010**: O sistema DEVE exibir a data associada de forma contextual: apenas o ano quando somente o ano estiver preenchido; mês e ano quando apenas esses dois campos estiverem preenchidos; data completa quando todos os campos estiverem preenchidos.
- **FR-011**: Os três campos (dia, mês, ano) DEVEM ser sempre visíveis no formulário de edição. Campos não preenchidos aparecem vazios (sem seleção). O campo de dia DEVE ficar desabilitado enquanto o mês não estiver selecionado, prevenindo combinações inválidas na própria interface.
- **FR-011a**: Ao remover o mês, o campo de dia DEVE ser automaticamente limpo em cascata, garantindo que nunca haja um dia salvo sem mês correspondente.
- **FR-012**: O seletor de dias DEVE se ajustar automaticamente ao número de dias válidos para o mês e ano selecionados (considerando anos bissextos).
- **FR-013**: O sistema DEVE oferecer um botão "Limpar data" que remove todos os campos da data associada de uma vez, deixando o documento sem data associada.
- **FR-014**: Ao ordenar documentos por data associada, datas parciais DEVEM ser tratadas como o início do período: ano-only equivale a 1º de janeiro do ano; mês+ano equivale ao 1º dia do mês.

### Key Entities

- **Documento**: Entidade central do sistema. Possui data de criação (imutável, gerada pelo sistema) e data associada (opcional, editável pelo usuário, com granularidade variável: ano, mês+ano ou dia+mês+ano).
- **Data Associada**: Estrutura composta por três campos independentes opcionais (dia, mês, ano), com apenas as combinações válidas permitidas. Representa um contexto temporal atribuído pelo usuário ao documento.
- **Data de Criação**: Data e hora exatas em que o documento foi criado no sistema. Imutável e nunca editável pelo usuário.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: O usuário consegue associar ou editar a data de um documento em menos de 30 segundos, sem necessidade de instrução prévia.
- **SC-002**: 100% dos documentos existentes têm a data associada corretamente pré-preenchida com a data de criação após a migração.
- **SC-003**: 0% de documentos salvos com combinações inválidas de data (ex.: dia sem mês).
- **SC-004**: A data de criação permanece inalterada em 100% dos documentos após qualquer operação de edição da data associada.
- **SC-005**: O seletor de datas apresenta apenas opções de dia válidas para o mês e ano selecionados, eliminando entradas inválidas sem necessidade de mensagens de erro ao selecionar.

---

## Assumptions

- A área de "Associações" já existe na interface do documento; esta funcionalidade adiciona a data associada dentro dessa área existente.
- O sistema já armazena e exibe a data de criação dos documentos; esta funcionalidade a torna explicitamente imutável e separada da data associada.
- O seletor de ano deve oferecer anos de 1900 até o ano atual + 10 anos futuros, permitindo registrar eventos planejados e metas futuras.
- Não há requisito de fuso horário diferenciado: a data associada é puramente calendárica (dia, mês, ano) sem componente de hora.
- A funcionalidade é aplicada a todos os documentos do usuário, sem distinção de tipo ou categoria.
- Documentos criados antes desta funcionalidade terão a data associada inicializada com a data de criação via migração de dados executada na implantação.
- A data associada não é obrigatória — um documento pode existir sem data associada.
