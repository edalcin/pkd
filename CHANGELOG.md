# Changelog

Todas as mudanças notáveis nesta aplicação.

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/).

## [Unreleased]

### Adicionado

- **Administração: desproteger documentos em lote** — nova aba "🛡️ Protegidos" lista todos os documentos criptografados em repouso (`GET /api/admin/protected`). O usuário seleciona um, vários, ou todos via checkbox e dispara uma única solicitação de código 2FA por e-mail (`POST /api/admin/unprotect/request`) que autoriza desproteger o lote inteiro de uma vez (`POST /api/admin/unprotect`) — mesmo padrão de geração de códigos de backup (solicita código → e-mail → confirma → executa). Antes, desproteção só era possível documento-a-documento, cada uma exigindo desbloquear o documento na sessão com um código próprio. Falha de decifração por documento (ex.: `PKD_PASSWORD` trocada após a proteção) não aborta o lote — o id entra em `failed` na resposta.

- **2FA por e-mail (login vinculado ao dispositivo) + criptografia de documentos** — dois recursos opt-in habilitados quando as quatro variáveis `SES_USERNAME`, `SES_PASSWORD`, `EMAIL_SENDER` e `EMAIL_2FA` estão definidas:
  - **Login com 2FA por e-mail:** após a senha mestra, se o navegador não for um dispositivo confiável, um código de 6 dígitos é enviado por e-mail (Amazon SES SMTP, `internal/email`); ao acertar o código o dispositivo é lembrado permanentemente (cookie `pkd_device`, tabela `trusted_devices`) e não é mais solicitado nesse navegador. Endpoint `POST /api/login/2fa`.
  - **Proteção de documento (criptografia em repouso):** ícone de escudo na barra de título alterna Proteger/Desproteger. Um documento protegido guarda o corpo como AES-256-GCM (`internal/security/crypto.go`; chave derivada de `PKD_PASSWORD`) na coluna `documents.encrypted`; título/ícone/tags continuam visíveis na árvore, mas o conteúdo some da busca full-text e dos embeddings semânticos enquanto protegido. Abrir o documento envia um novo código por e-mail; após validado, o corpo é decifrado e permanece aberto pelo resto da sessão do servidor (reinício do servidor tranca novamente). Endpoints `POST /api/documents/{id}/protect`, `/unprotect`, `/unlock/request`, `/unlock`.
  - **Administração:** botão "Esquecer dispositivos confiáveis" (`POST /api/admin/trusted-devices/forget`) revoga todos os dispositivos, exigindo código no próximo login em qualquer navegador; status do 2FA exposto em `GET /api/admin/settings` (`email_2fa_enabled`).
  - Novas variáveis de ambiente: `SES_USERNAME`, `SES_PASSWORD`, `EMAIL_SENDER`, `EMAIL_2FA`, `SES_HOST` (padrão `email-smtp.us-east-1.amazonaws.com`), `SES_PORT` (padrão `587`). Recurso totalmente opt-in — sem as quatro variáveis obrigatórias, o login permanece de etapa única, como antes.
  - ⚠️ Trocar `PKD_PASSWORD` torna documentos já protegidos indecifráveis (a chave é derivada da senha mestra) — é preciso desprotegê-los antes de rotacionar a senha.

- **Dashboard na Administração** — nova aba "📊 Início" como tela padrão da área administrativa:
  - Cards de resumo: total de documentos, arquivos associados, links e tags (via `GET /api/admin/stats`)
  - Cards de uso de disco: banco de dados, WAL (SQLite), arquivos associados e total — movidos da aba Storage para o Dashboard
  - Botão 🔄 atualiza stats e disco simultaneamente

- **Grafo semântico: clusters por comunidade Louvain** — ao ativar o toggle Semântico:
  - Correção de bug: `GET /api/graph?all=true` é chamado automaticamente no modo semântico, garantindo que todos os nós referenciados pelas arestas semânticas estejam presentes (antes o grafo retornava "0 nós · 0 arestas")
  - Algoritmo **Louvain local-moving** (nível único, JS puro, `frontend/src/lib/graph/community.js`) detecta comunidades no subgrafo de arestas semânticas visíveis; sem nova dependência npm
  - Nós coloridos por comunidade via ângulo dourado HSL; singletons em cinza (`#94a3b8`)
  - Layout inicial posiciona cada cluster em posição circular com forças `forceX`/`forceY` (strength 0.18); força `center` desativada no modo semântico
  - `toggleSemantic` chama `loadGraph()` ao ligar e ao desligar — garante refetch do conjunto correto de nós
  - Barra de status exibe `N nós · M arestas · K comunidades`
  - **Legenda de comunidades** (visível apenas no modo Semântico): checkbox por comunidade + "Todas" para marcar/desmarcar em bloco; visibilidade aplicada via D3 `display` sem reiniciar simulação física

- **Progresso em tempo real para Migrar / Reconciliar / Limpar origem** — as três operações de storage eram síncronas e bloqueavam o request sem feedback. Agora são assíncronas com job tracking (mesmo padrão do backup S3):
  - Endpoints renomeados: `/migrate-start`, `/reconcile-start`, `/cleanup-source-start` retornam `202 Accepted + job_id`; polling via `GET /api/admin/storage/jobs/{id}` existente.
  - Barra de progresso `<progress>` com `X / Y arquivos (N%)` durante execução.
  - Relatório final ao concluir: `total_found / copiados|corrigidos|removidos / erros`.
  - `BackupJobManager` passa a rastrear jobs de `kind = "migrate" | "reconcile" | "cleanup"` além de `"backup"`. Novo struct `StorageOpSummary` (total_found, succeeded, skipped, errors) como campo `Job.StorageOp`.
  - Callbacks `onProgress(processed, total int64)` adicionados a `MigrateToBackend`, `ReconcileStorageLocations` e `CleanupSource` em `store/attachments.go`.
  - Gate de concorrência: migrate/cleanup usam `currentBackendKind()` como chave; reconcile usa `reconcile-<backend>`.

- **Reconciliar LOCAL ↔ DB** — novo botão sempre visível (mesmo sem S3 configurado). Usa a mesma função `ReconcileStorageLocations` passando o backend local como origem. Útil após restore do DB quando os arquivos estão no disco mas o banco registra `storage_location = 's3'`.

### Corrigido

- **Limpar origem verifica cópia antes de apagar** — `CleanupSource` agora chama `target.Get(key)` para confirmar que o arquivo existe no backend ativo antes de deletá-lo da origem. Arquivos sem cópia confirmada são contados em `skipped` e **não são apagados**, prevenindo perda de dados. O relatório final exibe `candidatos / removidos / ignorados (sem cópia no destino)` com aviso orientando re-execução da migração se `skipped > 0`.

### Documentação

- `docs/operations.md` — seção "Migração, reconciliação e limpeza" reescrita para refletir comportamento assíncrono, novo botão LOCAL↔DB e semântica segura do Limpar origem.
- `README.md` — tabela de funcionalidades e seção "Armazenamento S3" atualizadas.
- `docs/c4/component.md` — descrições de `BackupJobManager`, `storage_handlers` e `AttachmentStore` atualizadas.

- **Importar imagens externas como attachments** — conteúdo colado de outros sites frequentemente contém `<img src="https://...">` externos que podem quebrar se o site original sair do ar.
  - **Editor**: botão `🌐⬇` na barra de ferramentas, visível apenas quando o documento aberto tem imagens externas. Um clique baixa todas as imagens, cria attachments inline e reescreve os `src` numa única transação ProseMirror (= um passo de undo).
  - **Admin → Arquivos**: nova seção "Imagens externas" lista todos os documentos com imagens externas (contagem por documento); botão "Importar" por linha. Após importação a seção e a grade de anexados são atualizadas automaticamente.
  - Endpoint `POST /api/documents/{id}/attachments/from-url` — baixa uma URL única e cria attachment inline; reutiliza `CreateFile` e respeita `PKD_MAX_IMAGE_MB`.
  - Endpoint `GET /api/admin/external-images` — varre `body_html` de todos os documentos não-deletados.
  - Endpoint `POST /api/admin/documents/{id}/import-external-images` — importa em lote, reescreve body e salva. Falhas individuais são não-fatais (contabilizadas em `failed`).
  - Segurança: `imagefetch.go` implementa fetch SSRF-safe com `net.Dialer.Control` que rejeita conexões a endereços privados, loopback, não-especificados, link-local, multicast e os CIDRs adicionais `169.254.0.0/16` (metadata cloud) e `100.64.0.0/10` (CGNAT), verificados após resolução DNS.

### Corrigido

- **Botão de importação de imagens externas** — comportamento corrigido e aprimorado:
  - Botão desabilitado durante a operação para evitar importações duplicadas por double-click.
  - Desaparece imediatamente após a importação quando não restam mais imagens externas (driven por `$derived.by` com `editorTick` como dependência reativa explícita, garantindo recálculo síncrono pós-dispatch).
  - Permanece visível e habilitado (com ícone ✓) quando a importação termina mas ainda existem imagens externas não importadas (ex: falhas parciais), permitindo retry.
  - Feedback inline na toolbar: mostra "N importada(s) / N falhou" por 5 segundos após a operação.
- **Editor não atualizava após import via Admin** — ao importar imagens pela aba Arquivos da Administração, o documento aberto no editor continuava exibindo os `src` externos (stale). Adicionado `docBodyRefreshedSignal` no store de documentos: Admin dispara o sinal após import; Editor escuta e atualiza o conteúdo via `editorInstance.commands.setContent` sem re-montar o editor nem recarregar a página.

- **Backup assíncrono de anexos com backend S3** (`005-s3-attachments-backup` US1)
  - Novo endpoint `POST /api/admin/storage/backup-start` cria job assíncrono que gera ZIP diretamente no bucket S3 sem materializar no disco da instância de aplicação (`io.Pipe` + `manager.Uploader` para multipart streaming).
  - ZIP gravado em prefixo reservado `_backup-tmp/<job-id>.zip`.
  - URL pré-assinada `GetObject` com TTL de 15 minutos para download direto S3 → navegador.
  - Endpoint `GET /api/admin/storage/jobs/{id}` para polling de status (running/succeeded/failed + progresso `processed/total`).
  - Endpoint `POST /api/admin/storage/jobs/{id}/download-url` para regenerar URL pré-assinada quando expira.
  - Manifesto interno `manifest.json` (última entrada) mapeia SHA256 → `stored_filename`s + tamanho + MIME, permitindo restauração backend-agnóstica.
  - Dedup natural por SHA256: anexos com mesmo conteúdo geram uma única entrada no ZIP.
  - Backfill automático de `content_sha256` em linhas que ainda não têm o hash (típico de uploads históricos no backend local).
  - Sweep não-bloqueante no startup remove ZIPs temporários órfãos com idade > 24h.
  - Tracking de jobs em memória com `BackupJobManager` (mutex + LRU cap 50, single-in-flight por backend, `ErrJobInFlight` → HTTP 409).

- **Restauração assíncrona cross-backend e in-place** (`005-s3-attachments-backup` US2/US3)
  - Novo endpoint `POST /api/admin/storage/restore-start` aceita ZIP do backup (mesmo formato local e S3).
  - Backend S3: stream-upload do ZIP recebido para `_backup-tmp/<job-id>-restore.zip` → `S3RangeReaderAt` (Range GETs como `io.ReaderAt`) → `archive/zip` → fan-out `Put` por `stored_filename`.
  - Backend local: temp file em `attachmentsDir` → `os.File` como `io.ReaderAt` → mesmo fluxo.
  - Cross-backend: ZIP de produção (S3) restaurado em desenvolvimento (local) e vice-versa.
  - In-place: ZIP restaurado no mesmo backend recupera arquivos perdidos.
  - 3 modos de conflito quando chave de destino já existe: `overwrite` (padrão), `keep`, `abort`.
  - Verificação de integridade SHA256 inline para cada entrada (per-entry failure isolation, FR-008).
  - Entradas órfãs (SHA256 sem linha correspondente em `attachments`) são ignoradas e listadas no resultado do job.
  - Cleanup do ZIP temporário garantido via `defer` + `recover()` em sucesso, falha ou panic.

- **Índice `idx_attachments_content_sha256`** para suportar lookup eficiente durante restauração.

- **UI Admin → Storage**: dois blocos novos quando backend ativo é S3 ("Backup assíncrono (S3)") e para qualquer backend ("Restauração assíncrona") com polling 2s, contadores, countdown da URL pré-assinada e lista colapsável de entradas ignoradas.

### Documentação

- `docs/operations.md` — seção nova "Backup e restauração de anexos (S3)" com fluxo de uso, IAM mínimo, sweep automático e regras de concorrência.
- `UNRAID.md` — nota sobre restauração cross-backend a partir de produção.

### Notas operacionais

- Política IAM mínima inclui agora `s3:DeleteObjects` (sweep) e `s3:AbortMultipartUpload`. Operações em buckets existentes podem precisar atualização da policy.
- Endpoints legados `/api/admin/storage/backup-attachments` (GET) e `/api/admin/storage/restore-attachments` (POST) continuam funcionando para o backend local. Marcados como legacy em comentário no código; remoção planejada para release futura após validação dos novos endpoints em produção.
