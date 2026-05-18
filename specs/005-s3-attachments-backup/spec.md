# Feature Specification: Backup & Restauração de Arquivos Associados com Backend S3

**Feature Branch**: `005-s3-attachments-backup` (spec armazenada em `main` — política do projeto não permite branches longas)
**Created**: 2026-05-18
**Status**: Draft
**Input**: User description: "Quero implementar o backup / restauração de arquivos associados quando o backend de armazenamento está no S3. Creio que a melhor opção é criar um ZIP com todos os arquivos no próprio S3 e ofertar para download. Não quero que o arquivo ZIP com os arquivos fique na instância EC2, onde roda a ferramenta. Considere que posso querer fazer backup de uma instância rodando na Amazon EC2 e com armazenamento no S3, como está o ambiente de produção; mas posso querer restaurar os arquivos associados no ambiente de desenvolvimento, rodando no UNRAID e armazenando localmente."

## Clarifications

### Session 2026-05-18

- Q: Comportamento de restauração para entradas do ZIP não referenciadas na base atual? → A: Restaurar apenas entradas cuja chave existe na tabela `attachments` atual; órfãs reportadas no log (não importadas).
- Q: TTL e modelo de acesso do link de download do ZIP no S3? → A: TTL de 15 minutos, URL re-gerável (nova requisição de download substitui a anterior); admin pode reacionar o backup para gerar nova URL se expirar.
- Q: Recuperação de crash — limpeza de artefatos temporários no S3? → A: ZIPs temporários gravados em prefixo dedicado `_backup-tmp/`. Sweep oportunista no startup da aplicação remove objetos com idade > 24h. Sem persistência adicional de estado de jobs.
- Q: Limite de tamanho por arquivo individual no ZIP? → A: Sempre gerar ZIP64; sem limite artificial por arquivo além dos limites do backend de origem/destino.
- Q: Identidade lógica das entradas no manifesto do ZIP? → A: SHA256 do conteúdo é a chave lógica. Backup MUST popular `content_sha256` para qualquer attachment que não tenha (computando ao ler o arquivo da origem). Restore localiza linhas via `SELECT ... WHERE content_sha256 = ?` e replica o conteúdo no backend de destino para cada linha encontrada (dedup natural no ZIP; fan-out no restore quando múltiplas linhas compartilham hash).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Backup completo dos arquivos com backend S3 (Priority: P1)

Administrador da instância de produção (EC2 + S3) precisa baixar um único arquivo ZIP contendo todos os arquivos anexados do sistema, sem que esse ZIP ocupe espaço temporário no disco da instância EC2 (que tem armazenamento limitado e efêmero).

**Why this priority**: É a única forma de garantir continuidade do conteúdo associado em caso de perda do bucket S3 ou para migração entre ambientes. Hoje, a funcionalidade de backup de arquivos só opera com backend local — produção fica sem recurso de backup integrado.

**Independent Test**: Em uma instância configurada com backend S3, acionar o backup pela interface administrativa e confirmar que (a) um ZIP é gerado contendo todos os arquivos referenciados na base, (b) o download é entregue ao navegador do administrador, e (c) nenhum arquivo temporário com o conteúdo agregado dos anexos é mantido no disco da instância da aplicação após o término da operação.

**Acceptance Scenarios**:

1. **Given** instância configurada com backend S3 e 50 arquivos anexados, **When** administrador aciona "Backup de arquivos", **Then** sistema disponibiliza um download contendo todos os 50 arquivos em um ZIP, sem que esse ZIP fique armazenado no disco local da aplicação após a operação concluir.
2. **Given** backup em andamento, **When** administrador acompanha o status, **Then** o sistema indica o progresso (ex.: número de arquivos processados / total) e o tamanho final do ZIP.
3. **Given** bucket S3 contém arquivos órfãos (não referenciados na base), **When** o backup é gerado, **Then** apenas os arquivos referenciados na base atual são incluídos no ZIP.
4. **Given** instância configurada com backend local, **When** administrador aciona "Backup de arquivos", **Then** a operação continua funcionando como antes (compatibilidade preservada).

---

### User Story 2 - Restauração cross-backend (S3 → Local) (Priority: P1)

Administrador deseja restaurar, em um ambiente de desenvolvimento com backend local (UNRAID, disco local), um ZIP de backup que foi gerado em outro ambiente com backend S3 (produção EC2).

**Why this priority**: Essencial para reproduzir o estado de produção em desenvolvimento, para debug, validação de migrações e treinamento. Sem isso, o backup gerado em produção é inútil em outros ambientes.

**Independent Test**: Gerar um ZIP de backup em uma instância S3, transferir o ZIP para uma instância distinta configurada com backend local, fazer upload via interface administrativa e confirmar que todos os arquivos ficam acessíveis pelo aplicativo (visualização e download dos anexos pelos documentos que os referenciam).

**Acceptance Scenarios**:

1. **Given** ZIP de backup gerado em ambiente S3 com 50 arquivos, **When** administrador faz upload do ZIP em instância com backend local, **Then** todos os 50 arquivos são restaurados no backend local e ficam acessíveis pelas referências existentes na base.
2. **Given** ZIP de backup, **When** restauração é executada, **Then** integridade de cada arquivo é validada (ex.: por hash registrado na base) antes da operação ser declarada bem-sucedida.
3. **Given** ZIP malformado ou corrompido, **When** upload é processado, **Then** sistema rejeita o arquivo com mensagem clara e não modifica o estado atual do backend.
4. **Given** backend local com arquivos existentes que coincidem com chaves do ZIP, **When** restauração é executada, **Then** administrador é informado e pode escolher entre sobrescrever ou abortar.

---

### User Story 3 - Restauração in-place (mesmo backend) (Priority: P2)

Administrador precisa restaurar um ZIP de backup no mesmo backend de onde foi gerado (S3 → S3 ou Local → Local), para recuperar de uma exclusão acidental ou para reverter o estado.

**Why this priority**: Cenário de recuperação direta. Importante, mas menos crítico que P1 — se P1 funciona, este caso é uma variação operacional.

**Independent Test**: Excluir alguns arquivos do backend ativo, restaurar via ZIP e confirmar que os arquivos voltam a estar disponíveis.

**Acceptance Scenarios**:

1. **Given** backend S3 com arquivos parcialmente perdidos, **When** administrador restaura ZIP de backup anterior, **Then** arquivos ausentes são recolocados no S3 sem que o ZIP transite pelo disco da instância EC2.
2. **Given** backend local com arquivos perdidos, **When** ZIP de backup é restaurado, **Then** arquivos são repostos no diretório local.

---

### Edge Cases

- **Volume grande de arquivos**: bucket com volumes na ordem de gigabytes — operação deve permanecer estável sem esgotar memória ou disco da instância de aplicação.
- **Arquivo individual grande**: anexo único com várias centenas de MB ou maior que 4 GB — deve ser incluído no ZIP sem materialização integral em memória, usando ZIP64 para entradas acima do teto clássico de 4 GB.
- **Bucket inacessível durante operação**: falha de rede ou credencial revogada no meio do processo — operação deve abortar de forma limpa, sem deixar estado parcial visível ao usuário final.
- **Sessão do administrador expira durante operação longa**: download/upload longo não deve ser invalidado por timeout de sessão da UI.
- **ZIP de restauração contém arquivos não referenciados na base atual**: sistema ignora a entrada órfã (não escreve no backend) e registra a chave no log da operação como "skipped: no matching attachment row".
- **Backup acionado simultaneamente por múltiplos administradores**: comportamento concorrente precisa estar definido (fila, recusar segundo pedido, ou permitir).
- **Espaço insuficiente no destino S3**: bucket sem capacidade ou política de tamanho — sistema deve reportar erro claro.
- **Hash do arquivo na base não bate com o conteúdo restaurado**: integridade comprometida deve ser sinalizada por arquivo, sem abortar a operação inteira.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Sistema MUST oferecer ação administrativa de "Backup de arquivos" funcional independentemente do backend de armazenamento ativo (local ou S3).
- **FR-002**: Quando o backend ativo for S3, o sistema MUST produzir o ZIP de backup utilizando o próprio S3 como área de materialização e/ou streaming, sem que o conteúdo agregado dos arquivos resida no disco local da instância de aplicação após o término da operação.
- **FR-003**: Sistema MUST disponibilizar o ZIP gerado para download pelo administrador através de mecanismo que não exija que o conteúdo do ZIP passe pelo disco da instância de aplicação (ex.: link direto ao objeto no S3 com validade limitada). O link MUST ter TTL de **15 minutos**. Uma nova requisição de download dentro da mesma sessão de backup MUST gerar nova URL (substituindo a anterior). Após expirar, admin pode reacionar a operação de backup para gerar novo ZIP e nova URL.
- **FR-004**: ZIP de backup MUST conter todos os arquivos referenciados pela base de dados ativa, identificados de forma que permitam restauração em qualquer backend suportado.
- **FR-005**: ZIP de backup MUST incluir, junto aos arquivos, metadados mínimos necessários para validar integridade na restauração (ex.: manifesto com chave lógica, tamanho e hash de cada arquivo).
- **FR-006**: Sistema MUST oferecer ação administrativa de "Restauração de arquivos" que aceite ZIP gerado pela funcionalidade de backup, independentemente do backend ativo no momento da restauração.
- **FR-007**: Restauração MUST funcionar cross-backend: ZIP produzido em ambiente com backend S3 deve poder ser restaurado em ambiente com backend local, e vice-versa.
- **FR-008**: Restauração MUST validar integridade de cada arquivo restaurado contra o hash registrado e relatar discrepâncias por arquivo, sem interromper a operação inteira por falha individual.
- **FR-009**: Sistema MUST limpar qualquer artefato temporário (objeto auxiliar no S3, arquivo temporário em disco) criado durante backup ou restauração, independentemente de a operação ter sucesso ou falhar.
- **FR-009a**: ZIPs temporários no S3 MUST ser gravados em um prefixo reservado dedicado (ex.: `_backup-tmp/`). Esse prefixo MUST conter apenas artefatos transitórios de backup; conteúdo "real" de anexos NUNCA deve usar esse prefixo.
- **FR-009b**: Na inicialização da aplicação (quando backend ativo for S3), sistema MUST executar varredura no prefixo de ZIPs temporários e remover objetos com idade > 24 horas (recuperação de crashes de operações anteriores que não puderam concluir limpeza inline).
- **FR-010**: Sistema MUST exigir privilégios administrativos para iniciar backup ou restauração.
- **FR-011**: Sistema MUST registrar (log) início, fim e resultado de cada operação de backup e restauração, incluindo identidade do administrador, contagem de arquivos processados, tamanho total e duração.
- **FR-012**: Sistema MUST informar o administrador, com mensagens claras, sobre falhas em qualquer etapa (geração, transferência, restauração, validação) — incluindo causa quando determinável (ex.: credencial inválida, bucket inacessível, espaço esgotado, ZIP inválido).
- **FR-013**: Backup MUST capturar apenas arquivos referenciados pela base atual; arquivos órfãos no backend NÃO devem ser incluídos.
- **FR-014**: Restauração MUST definir comportamento explícito quando o backend de destino já contém um arquivo com chave coincidente (opções suportadas: sobrescrever, manter existente, abortar — escolha exposta ao administrador antes da execução).
- **FR-015**: Operações de backup e restauração MUST ser serializadas — sistema NÃO deve permitir duas operações simultâneas no mesmo backend; segunda requisição concorrente deve ser rejeitada com mensagem clara.
- **FR-016**: Sistema MUST permitir que o administrador acompanhe o progresso de operações longas (contagem de arquivos processados / total, status atual).
- **FR-017**: Restauração MUST ignorar entradas do ZIP cuja chave lógica (SHA256) não corresponda a nenhuma linha da tabela `attachments` no momento da operação. Cada entrada ignorada MUST ser registrada no log da operação (SHA256, tamanho, motivo "no matching attachment row"). Backend de destino NÃO deve receber escrita para entradas órfãs.
- **FR-018**: ZIP de backup MUST usar o formato ZIP64 sempre (independentemente de tamanho), eliminando o teto de 4 GB por entrada e 4 GB por arquivo total da especificação ZIP clássica. Não há limite artificial por arquivo individual além do que o backend de origem/destino impõe.
- **FR-019**: Chave lógica do manifesto MUST ser o **SHA256 do conteúdo** do arquivo. Cada entrada do ZIP é nomeada/indexada por esse hash. Manifesto MUST registrar, por entrada: SHA256, tamanho em bytes, MIME type (quando conhecido), e a lista de `stored_filename`s da tabela `attachments` que apontam para aquele conteúdo (suporte a dedup: múltiplas linhas podem compartilhar hash).
- **FR-020**: Backup MUST garantir que `content_sha256` esteja populado para toda linha de `attachments` incluída. Quando a coluna estiver nula (típico de uploads históricos no backend local), backup MUST computar o hash ao ler o arquivo da origem e persistir o valor na base antes de continuar.
- **FR-021**: Restore MUST localizar linhas de destino via `SELECT ... FROM attachments WHERE content_sha256 = ?` usando a chave de cada entrada do ZIP. Para cada linha encontrada, conteúdo MUST ser escrito no backend ativo usando a chave de storage que aquela linha referencia (`stored_filename` resolvido pela convenção do backend ativo). Se a mesma chave SHA256 corresponde a N linhas, conteúdo é escrito N vezes (uma por linha).

### Key Entities *(include if feature involves data)*

- **Backup ZIP**: pacote contendo o conteúdo de todos os arquivos anexados referenciados pela base, acompanhado de um manifesto interno. Cada entrada do ZIP é nomeada pelo SHA256 do conteúdo (chave lógica), garantindo identidade backend-agnóstica e dedup natural quando múltiplas linhas de `attachments` compartilham o mesmo conteúdo.
- **Job de Backup/Restauração**: operação administrativa com identificador único, estado (em andamento, concluída, falhou), contadores de progresso, identidade do administrador, timestamps de início e fim, e referências aos artefatos temporários para limpeza.
- **Manifesto do ZIP**: estrutura interna ao pacote que descreve cada entrada (SHA256, tamanho em bytes, MIME type, lista de `stored_filename`s da tabela `attachments` que referenciam aquele conteúdo). Permite validação na restauração, suporte cross-backend e fan-out quando uma mesma entrada deve ser escrita em múltiplas chaves de storage.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Administrador em ambiente de produção (EC2 + S3) consegue gerar e baixar um ZIP de backup contendo a totalidade dos arquivos anexados sem que o disco da instância de aplicação seja utilizado para armazenar o conteúdo agregado do ZIP em momento algum.
- **SC-002**: ZIP gerado em ambiente S3 pode ser restaurado com 100% de sucesso em ambiente com backend local; após a restauração, 100% dos anexos previamente acessíveis voltam a estar acessíveis pelas referências da base.
- **SC-003**: 100% dos arquivos restaurados têm sua integridade verificada contra o hash registrado e o resultado é relatado por arquivo no log da operação.
- **SC-004**: Operação de backup conclui com sucesso para volumes de até 10 GB de arquivos sem requerer aumento de recursos de memória ou disco da instância de aplicação além dos limites atualmente provisionados.
- **SC-005**: Em caso de falha em qualquer etapa, 100% dos artefatos temporários (objetos auxiliares no S3, arquivos temporários em disco) são removidos automaticamente — verificação manual confirma ausência de lixo após cenários de falha simulados. Em caso de crash da aplicação durante a operação, próximo startup remove órfãos com idade > 24h no prefixo de ZIPs temporários.
- **SC-006**: Administrador identifica a causa de uma falha em menos de 1 minuto a partir da mensagem exibida e do log da operação, sem necessidade de consultar logs de baixo nível da infraestrutura.

## Assumptions

- O backup e a restauração cobrem **apenas arquivos anexados** (binários armazenados no backend de storage). O backup do banco de dados continua sendo uma operação separada já existente; a integração de um "backup completo unificado" está fora do escopo desta feature.
- A funcionalidade só é exposta na interface administrativa existente, exigindo autenticação de administrador.
- O bucket S3 utilizado para backup é o mesmo bucket atualmente configurado como backend de armazenamento; não há nova credencial nem novo bucket dedicado.
- O download do ZIP de backup é feito diretamente pelo administrador a partir do navegador, em uma única requisição (não há mecanismo de retomada de download parcial nesta versão).
- A funcionalidade de upload do ZIP de restauração aceita arquivos transmitidos via interface administrativa; o limite prático do tamanho do ZIP suportado em uma única operação é o que o ambiente de execução já suporta para uploads administrativos.
- Restauração in-place (mesmo backend) e cross-backend (S3 ↔ local) usam o **mesmo formato de ZIP** e o **mesmo manifesto interno** — não há formato distinto por backend.
- Operações são síncronas do ponto de vista do administrador: ele permanece na interface enquanto a operação executa, com indicador de progresso. Execução em background com notificação posterior está fora do escopo.
- Permissões IAM da instância de aplicação no S3 já incluem (ou serão atualizadas operacionalmente para incluir) as ações necessárias para criar, listar, gerar URL pré-assinada e excluir objetos no bucket — não há gestão de credenciais adicional nesta feature.
- Conflitos de chave durante restauração (FR-014) usam, por padrão, a opção **"sobrescrever"** se o administrador não escolher explicitamente — mantendo o princípio de "restauração restaura".
