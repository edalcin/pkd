---
description: "Task list for 005-s3-attachments-backup"
---

# Tasks: Backup & Restauração de Arquivos Associados com Backend S3

**Input**: Design documents from `/specs/005-s3-attachments-backup/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api.yaml, quickstart.md

## ⏸ Resume here

**Última atualização**: 2026-05-18
**Estado**: US1 (Phase 1+2+3) **entregue** em commit `52be95f` (`origin/main`). Imagem `:edge` em build via workflow `Build and Publish`.

**Retomar em**: Phase 4 (US2 — restore cross-backend) começando por T032.

**Antes de prosseguir** (foundational adicional não coberto em US1):
- Re-implementar `GetRange(ctx, key, offset, length) ([]byte, error)` e `HeadSize(ctx, key) (int64, error)` na interface `S3Capable` em `internal/storage/storage.go` + impl em `internal/storage/s3.go` (foram revertidos no PR de US1 para manter escopo limpo — ver `git log --diff-filter=D` para o code-shape).
- T007: `LookupBySHA256(ctx, sha) ([]AttachmentRef, error)` em `internal/store/attachments.go` (usa `idx_attachments_content_sha256` já criado em T005).

**Validação que US1 está em produção** (precondição para mexer em restore):
- Confirmar que admin gerou e baixou um ZIP de backup S3 via UI sem incidentes
- Confirmar que sweep removeu temp object > 24h pelo menos uma vez (log da aplicação)

**Comando para retomar**: abrir esta linha e seguir Phase 4 em ordem. Marcar `[x]` por task. Após Phase 4 → commit `feat(restore): restauração cross-backend de anexos (US2)`. Após Phase 5 → `feat(restore): restauração in-place (US3)`. Após Phase 6 → `docs(backup): operacional + perf + IAM (Polish)`.

**Memory marker**: ver `~/.claude/projects/D--git-pkd/memory/project_s3_backup_resume.md`.

---

**Tests**: Tests INCLUDED — projeto tem `tests/contract`, `tests/integration`, `tests/unit` ativos no CI (build-and-publish.yml). Test tasks devem ser escritas **antes** das tasks de implementação correspondentes (preserva regressão zero).

**Organization**: Tasks agrupadas por user story (US1 Backup S3 P1; US2 Restore cross-backend P1; US3 Restore in-place P2). Cada story é independentemente testável e entregável.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: arquivo distinto, sem dependência em task incompleta → paralelizável
- **[Story]**: US1 / US2 / US3
- Setup e Foundational sem label de story; Polish idem

## Path Conventions

- Backend Go: `internal/...`, `cmd/pkd/...`
- Frontend: `frontend/src/lib/components/...`
- Tests: `tests/contract`, `tests/integration`, `tests/unit`
- Specs: `specs/005-s3-attachments-backup/...`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: dependências e esqueleto de pacotes/arquivos.

- [x] T001 Adicionar `github.com/aws/aws-sdk-go-v2/feature/s3/manager` em `D:\git\pkd\go.mod` via `go get github.com/aws/aws-sdk-go-v2/feature/s3/manager@latest` (compatível com `service/s3 v1.101.0`); rodar `go mod tidy` — *dep adicionada; `tidy` removeu pois ainda sem imports; será re-adicionada automaticamente quando código importar (T023)*
- [x] T002 [P] Criar pacote `D:\git\pkd\internal\backup\` com arquivos vazios: `manifest.go`, `writer.go`, `reader.go`, `sweep.go` (apenas `package backup`)
- [x] T003 [P] Criar arquivo `D:\git\pkd\internal\server\jobs.go` (apenas `package server`)
- [x] T004 [P] Criar diretório `D:\git\pkd\specs\005-s3-attachments-backup\contracts\` se ausente (já criado em /speckit.plan) — verificar presença de `api.yaml`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: tipos, interfaces, migrações e plumbing que TODAS as user stories dependem.

**⚠️ CRITICAL**: Nenhuma task US1/US2/US3 pode iniciar antes deste checkpoint.

- [x] T005 Adicionar migração `CREATE INDEX IF NOT EXISTS idx_attachments_content_sha256 ON attachments(content_sha256)` em `D:\git\pkd\internal\store\migrate.go` (apêndice à lista de migrações existentes, segue padrão atual)
- [x] T006 [P] Implementar `AttachmentForBackup` struct + `EnumerateForBackup(ctx) ([]AttachmentForBackup, error)` em `D:\git\pkd\internal\store\attachments.go` (SELECT batch sobre tabela `attachments`)
- [ ] T007 [P] Implementar `AttachmentRef` struct + `LookupBySHA256(ctx, sha) ([]AttachmentRef, error)` em `D:\git\pkd\internal\store\attachments.go` (query usa novo índice T005) — *escopo restore (US2), fora do MVP*
- [x] T008 [P] Implementar `BackfillSHA256(ctx, attachmentID int64, sha string) error` em `D:\git\pkd\internal\store\attachments.go` (UPDATE simples)
- [x] T009 [P] Adicionar validador defensivo em `CreateFile` de `D:\git\pkd\internal\store\attachments.go` rejeitando `stored_filename` com prefixo `_backup-tmp/` (retorna erro claro; defesa em profundidade contra colisão com prefixo reservado)
- [x] T010 [P] Definir interface `S3Capable` (`PresignGet`, `UploadFromReader`, `ListWithMetadata`, `DeleteMany`) + struct `ObjectMeta` em `D:\git\pkd\internal\storage\storage.go` — *`GetRange` adiado (só restore)*
- [x] T011 Implementar métodos `S3Capable` no struct `S3Backend` em `D:\git\pkd\internal\storage\s3.go` (UploadFromReader via `manager.Uploader`; PresignGet via `s3.NewPresignClient`; ListWithMetadata via paginator; DeleteMany batch 1000)
- [ ] T012 [P] Definir interface mínima `S3API` em `D:\git\pkd\internal\storage\s3.go` — *deferido; testes unit do writer usam fakeSource em vez de mock S3*
- [x] T013 [P] Implementar `Manifest`, `EnvDescriptor`, `ManifestEntry` + JSON encode/decode + validação em `D:\git\pkd\internal\backup\manifest.go`
- [x] T014 Implementar `Job` e `BackupJobManager` (Start, Get, Finish, LRU manual cap=50, `ErrJobInFlight`) em `D:\git\pkd\internal\server\jobs.go`
- [x] T015 Wire `BackupJobManager` no struct `Server` em `D:\git\pkd\internal\server\server.go`
- [x] T016 Adicionar helper `currentBackendKind() string` em `D:\git\pkd\internal\server\server.go`

**Checkpoint**: Foundation pronto. Tasks US1/US2/US3 podem iniciar (em paralelo se equipe permitir; US1 e US2 são P1).

---

## Phase 3: User Story 1 — Backup com backend S3 (Priority: P1) 🎯 MVP

**Goal**: Admin em ambiente S3 gera ZIP contendo todos anexos referenciados, sem materializar no disco da app, e baixa via URL pré-assinada (TTL 15min).

**Independent Test**: rodar quickstart.md cenário 1; confirmar (a) job conclui succeeded; (b) `du` no disco da app permanece constante; (c) bucket S3 contém `_backup-tmp/<id>.zip`; (d) URL pré-assinada permite download do ZIP que contém `manifest.json` + uma entrada por SHA256 único.

### Tests for User Story 1 ⚠️

> Escrever testes FIRST. Devem FALHAR antes da implementação.

- [x] T017 [P] [US1] Unit test encode/decode + validação de Manifest em `D:\git\pkd\tests\unit\manifest_test.go` (4 testes: round-trip, version reject, SHA reject, empty stored_filenames reject)
- [x] T018 [P] [US1] Unit test `BackupJobManager` em `D:\git\pkd\tests\unit\jobs_test.go` (4 testes: single in-flight, Finish→history, Get unknown, SetDownloadURL)
- [ ] T019 [P] [US1] Integration test backup local backend — *novo handler retorna 501 para local (escopo MVP); não aplicável*
- [x] T020 [P] [US1] Unit test StreamingBackup end-to-end em `D:\git\pkd\tests\unit\backup_writer_test.go` com `fakeSource` em memória (cobre: round-trip ZIP, dedup por SHA256, backfill SHA256 quando ausente, manifest na última entrada, integrity check inline) — *escolhido unit + fakeSource em vez de integration test com mock S3, ver T012*
- [x] T021 [P] [US1] Backfill SHA256 coberto por T020 (cenário com `ContentSHA256: ""` força backfill verificado via `recordingPersister`)
- [ ] T022 [P] [US1] Contract test — *deferido; suite contract existente continua verde, novos endpoints validados manualmente via quickstart*

### Implementation for User Story 1

- [x] T023 [US1] Implementar `StreamingBackup(ctx, sink io.Writer, opts WriteOptions) (Result, error)` em `D:\git\pkd\internal\backup\writer.go`:
  - Enumera anexos via `attStore.EnumerateForBackup`
  - Agrupa por SHA256; para linhas sem SHA, primeira passada baixa+computa+backfill (descarta conteúdo)
  - Segunda passada: cria `zip.NewWriter(sink)`; para cada SHA único faz `srcBackend.Get(stored_filename)` (ou `GetRange` se S3Capable e tamanho conhecido) → `zip.CreateHeader(name=sha256)` → `io.Copy` com `io.TeeReader` para verificação inline (defensive double-check)
  - Última entrada: `manifest.json` serializado de `Manifest{Version:1, Entries: agrupadas}`
  - Retorna `Result{Total, Processed, SizeBytes}` (Job atualiza via callback)
  - Suporta callback `OnProgress(processed int64)` para atualização de Job.Processed
- [x] T024 [US1] Implementar `SweepStaleTempObjects` em `D:\git\pkd\internal\backup\sweep.go`
- [x] T025 [US1] Wire sweep no startup em `D:\git\pkd\internal\server\backup_sweep.go` (goroutine non-blocking; chamado de `New()`)
- [x] T026 [US1] Implementar `handleAdminStorageBackupStart` em `D:\git\pkd\internal\server\handlers_admin_storage_jobs.go`:
  - Autoriza admin
  - `kind := s.currentBackendKind()`
  - `job, err := s.jobs.Start("backup", kind, adminID)`; se `ErrJobInFlight` → 409
  - Dispatch goroutine: gera tempKey `_backup-tmp/<job.ID>.zip`
    - **S3 path** (`S3Capable`): `io.Pipe()`; goroutine A roda `backup.StreamingBackup(..., pw)`; goroutine B roda `cap.UploadFromReader(tempKey, pr, "application/zip")`; join via channel de erros
    - **Local path**: cria temp file no diretório do backend local; `backup.StreamingBackup(..., file)`; mantém temp file para servir GET (handler legado mantém comportamento atual de stream-to-response no caminho local; novo handler para local apenas marca job done com URL relativa `/api/admin/storage/jobs/<id>/download` se quisermos unificar — **decisão: backend local continua a usar endpoint legado direto-stream nesta entrega; novo handler retorna 501 se backend não é S3** — simplifica escopo)
    - Se S3 path concluiu OK: `cap.PresignGet(tempKey, 15min)` → seta `job.DownloadURL` e `job.URLExpiresAt`
    - Em qualquer erro: best-effort `cap.Delete(tempKey)`; `s.jobs.finishLocked(job, "failed", err.Error())`
    - Em sucesso: `s.jobs.finishLocked(job, "succeeded", "")`
  - Responde 202 + JSON `{ "job_id": job.ID }`
- [x] T027 [US1] Implementar `handleAdminStorageGetJob` em `D:\git\pkd\internal\server\handlers_admin_storage_jobs.go`
- [x] T028 [US1] Implementar `handleAdminStorageRegenerateDownloadURL` em `D:\git\pkd\internal\server\handlers_admin_storage_jobs.go`:
  - Recupera job; valida `kind == "backup"` e `backendKind == "s3"` (senão 409); valida `state == "succeeded"` (senão 404)
  - Recalcula tempKey `_backup-tmp/<job.ID>.zip`; verifica `cap.GetRange(tempKey, 0, 1)` para confirmar que objeto ainda existe — se erro NotFound → 404 "temp object expired or removed"
  - `cap.PresignGet(tempKey, 15min)` → atualiza job; retorna `{ download_url, url_expires_at }`
- [x] T029 [US1] Registrar rotas em `D:\git\pkd\internal\server\server.go`
- [x] T030 [US1] Estender UI admin em `D:\git\pkd\frontend\src\lib\components\Admin.svelte` Storage tab:
  - Novo bloco "Backup assíncrono (S3)" visível quando `storageStatus.kind === 's3'`
  - Botão "Iniciar backup" → POST `/api/admin/storage/backup-start` → guarda `jobId` em estado local
  - Função `pollJob(id)`: GET `/api/admin/storage/jobs/{id}` a cada 2s enquanto `state === 'running'`; exibir progresso (`processed`/`total`); ao chegar `succeeded` exibir botão "Baixar ZIP" linkando para `download_url` e contagem regressiva até `url_expires_at`
  - Botão "Gerar nova URL" → POST `.../jobs/{id}/download-url` (se URL expirou)
  - Em `failed` exibir `error_message`
- [x] T031 [US1] Mensagens PT-BR adicionadas em `Admin.svelte` (alert 409, link expirado com botão "Gerar nova URL")

**Checkpoint**: US1 funcional. Backup S3 produzindo ZIP downloadável via URL pré-assinada. Smoke test do quickstart.md cenário 1 passa.

---

## Phase 4: User Story 2 — Restauração cross-backend S3 → Local (Priority: P1)

**Goal**: ZIP gerado em ambiente S3 (prod) é restaurado em instância com backend local (dev), recolocando todos anexos.

**Independent Test**: rodar quickstart.md cenário 2; confirmar (a) job conclui succeeded; (b) diretório local backend contém arquivos com hashes batendo no `content_sha256` da base dev; (c) `skipped_entries` vazio se DBs sincronizadas, populado se base dev tem subset.

### Tests for User Story 2 ⚠️

- [ ] T032 [P] [US2] Unit test `StreamingRestore` em `D:\git\pkd\tests\unit\restore_test.go`:
  - ZIP válido + manifesto + entradas que batem em mock `LookupBySHA256` → escreve no mock backend
  - ZIP com entrada cujo SHA não bate em DB → `SkippedEntries` populado, nada escrito
  - ZIP com integridade quebrada (conteúdo alterado, hash não bate com nome da entrada) → entrada reportada como erro por arquivo, sem abortar
  - ZIP sem `manifest.json` ou versão != 1 → retorna erro claro
- [ ] T033 [P] [US2] Integration test restore S3-generated ZIP → Local backend em `D:\git\pkd\tests\integration\backup_restore_cross_test.go`:
  - Setup: criar `attachments` rows em DB de teste com hashes conhecidos
  - Gerar ZIP via `StreamingBackup` numa instância (backend mock S3); persistir bytes
  - Restaurar via `StreamingRestore` numa segunda instância (backend local em tempdir); confirmar arquivos restaurados
- [ ] T034 [P] [US2] Integration test fan-out: duas linhas `attachments` com mesmo `content_sha256` (mesmo conteúdo, stored_filenames distintos) → restore escreve duas vezes no backend em `D:\git\pkd\tests\integration\backup_restore_fanout_test.go`
- [ ] T035 [P] [US2] Contract test endpoint `/api/admin/storage/restore-start` em `D:\git\pkd\tests\contract\openapi_test.go` (estender existente) — adicionar paths em `D:\git\pkd\tests\contract\openapi.yaml` antes

### Implementation for User Story 2

- [ ] T036 [US2] Implementar `s3RangeReaderAt` adapter em `D:\git\pkd\internal\backup\reader.go`:
  - Struct embeds `S3Capable` + key + size
  - `ReadAt(p []byte, off int64) (n int, err error)` chama `cap.GetRange(ctx, key, off, int64(len(p)))` e copia para `p`
  - Cache mínimo opcional (read-ahead 64KB) para reduzir N de Range requests; v1 pode ser direto sem cache
- [ ] T037 [US2] Implementar `StreamingRestore(ctx, zipSrc io.ReaderAt, size int64, manifest *Manifest, attStore Lookup, dst Backend, opts RestoreOptions) (Result, error)` em `D:\git\pkd\internal\backup\reader.go`:
  - `zip.NewReader(zipSrc, size)`
  - Decodifica `manifest.json` (última entrada)
  - Para cada `entry`: `refs := attStore.LookupBySHA256(entry.SHA256)`; se vazio → `SkippedEntries` += {SHA, size, "no matching attachment row"}; continue
  - Encontra entrada do ZIP pelo nome `entry.SHA256`; abre; lê todo conteúdo em buffer (ou stream se size > threshold)
  - Verifica `sha256(content) == entry.SHA256`; se não, registra erro por entrada e continua (FR-008)
  - Para cada `ref`: aplica `opts.OnConflict` (overwrite/keep/abort) via `dst.Exists?` (interface helper) + `dst.Put`
  - Atualiza `Result.Processed` por escrita
- [ ] T038 [US2] Implementar `RestoreOptions{OnConflict string}` + helper `applyConflict(dst Backend, key string, mode string) (skip bool, err error)` em `D:\git\pkd\internal\backup\reader.go`. Adicionar método `Exists(ctx, key)` ao Backend interface se ainda não existir (ou usar `Get` com erro NotFound como sinal)
- [ ] T039 [US2] Implementar `handleAdminStorageRestoreStart(w, r)` em `D:\git\pkd\internal\server\handlers_admin_storage.go`:
  - Autoriza admin
  - `r.ParseMultipartForm(<limite atual de upload admin>)`; obtém file `zip` e form value `on_conflict` (default "overwrite")
  - `kind := s.currentBackendKind()`
  - `job, err := s.jobs.Start("restore", kind, adminID)`; ErrJobInFlight → 409
  - Dispatch goroutine:
    - **S3 path**: `tempKey := "_backup-tmp/<job.ID>-restore.zip"`; `cap.UploadFromReader(tempKey, file, "application/zip")`; size via HEAD ou via `cap.ListWithMetadata`; `zipSrc := newS3RangeReaderAt(cap, tempKey, size)`
    - **Local path**: cria temp file no `os.CreateTemp(localBackendDir, "restore-*.zip")`; `io.Copy(temp, file)`; `zipSrc := temp` (já é `io.ReaderAt`)
    - `StreamingRestore(...)` com callback de progresso atualizando `job.Processed`/`job.Total`
    - Cleanup do temp ZIP (S3 `cap.Delete` ou `os.Remove`) — defer em ambos caminhos
    - `s.jobs.finishLocked(job, state, errMsg)`; `job.SkippedEntries = result.SkippedEntries`
  - Responde 202 + `{ job_id }`
- [ ] T040 [US2] Registrar rota POST `/api/admin/storage/restore-start` em `D:\git\pkd\internal\server\server.go`
- [ ] T041 [US2] Estender UI admin em `D:\git\pkd\frontend\src\lib\components\Admin.svelte`:
  - Novo bloco "Restauração assíncrona" (substitui ou complementa botão atual de restauração local)
  - File input para ZIP; select para `on_conflict` (Sobrescrever/Manter existente/Abortar)
  - Submit → POST `/api/admin/storage/restore-start` multipart → `pollJob(jobId)`
  - Exibir `skipped_entries` ao final como lista colapsável ("N entradas órfãs ignoradas")

**Checkpoint**: US2 funcional. Restauração cross-backend e in-place compartilham mesmo handler. Quickstart cenário 2 passa.

---

## Phase 5: User Story 3 — Restauração in-place mesmo backend (Priority: P2)

**Goal**: ZIP restaurado no mesmo backend de origem (S3→S3 ou Local→Local), recuperando arquivos perdidos.

**Independent Test**: rodar quickstart.md cenário 3; confirmar (a) arquivos removidos voltam; (b) backend S3: temp object `_backup-tmp/<id>-restore.zip` é removido ao final; (c) disco da app permanece intocado no caso S3.

> US3 reusa quase totalmente o código de US2. Estas tasks são apenas validação dedicada + casos não cobertos.

### Tests for User Story 3 ⚠️

- [ ] T042 [P] [US3] Integration test restore S3 → S3 (in-place) em `D:\git\pkd\tests\integration\backup_restore_inplace_s3_test.go`:
  - Gera ZIP via backup S3; deleta alguns objetos do bucket; restaura mesmo ZIP no mesmo bucket; confirma objetos voltam; confirma temp object é removido
- [ ] T043 [P] [US3] Integration test restore Local → Local (in-place) em `D:\git\pkd\tests\integration\backup_restore_inplace_local_test.go`
- [ ] T044 [P] [US3] Integration test cleanup pós-falha: simular erro durante restore (mock backend `Put` falhar no meio); confirmar (a) job termina `failed`; (b) temp ZIP S3 é removido apesar da falha; em `D:\git\pkd\tests\integration\backup_restore_failure_cleanup_test.go`

### Implementation for User Story 3

- [ ] T045 [US3] Auditar `handleAdminStorageRestoreStart` (T039): verificar via code review que `defer cleanupTempZip(...)` executa em **todos** caminhos (sucesso, erro de parse, erro de StreamingRestore, panic recovered). Adicionar `recover()` no goroutine handler se ainda não houver, garantindo cleanup mesmo em panic
- [ ] T046 [US3] Adicionar contador no log estruturado: quantos refs foram restaurados, quantos foram pulados por conflito (keep mode), quantos por hash mismatch. Modifica `Job.Result` para incluir `WrittenCount`, `KeptCount`, `HashMismatchCount` em `D:\git\pkd\internal\backup\reader.go` + `D:\git\pkd\internal\server\jobs.go`

**Checkpoint**: US3 validado. Cenários in-place + failure cleanup cobertos por testes.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T047 [P] Adicionar seção "Backup/Restauração de anexos (S3)" em `D:\git\pkd\docs\` (ou criar `D:\git\pkd\docs\backup-restore.md` se diretório não tem README dedicado); cobrir fluxo de uso, IAM mínimo, troubleshooting (referência cruzada com quickstart.md)
- [ ] T048 [P] Atualizar `D:\git\pkd\UNRAID.md` com nota: "restauração cross-backend desde produção (S3) suportada via UI admin → Storage tab"
- [ ] T049 Atualizar comentário no topo de `D:\git\pkd\internal\server\handlers_admin_storage.go` listando endpoints legados (`/backup-attachments`, `/restore-attachments`) como **mantidos para backend local até v1.1**; endpoints novos são preferidos
- [ ] T050 [P] Run quickstart.md completo (cenários 1, 2, 3, e tabela de falhas) com minio local; documentar quaisquer desvios em commit message
- [ ] T051 [P] Stress test: gerar 100 anexos sintéticos (10MB cada = 1 GB total) num bucket de teste; medir tempo de backup e pico de heap (`go tool pprof`). Confirmar heap < 50 MB durante operação (SC alvo). Documentar resultado em comentário do PR ou em `D:\git\pkd\specs\005-s3-attachments-backup\notes\perf.md`
- [ ] T052 [P] Failure injection manual: revogar `PKD_S3_SECRET_ACCESS_KEY` durante backup em andamento; confirmar job termina `failed` e temp object é tentado limpar (best-effort, falhará devido a credencial — registrar no log para sweep do startup)
- [ ] T053 Atualizar `D:\git\pkd\CLAUDE.md` Recent Changes (já feito em /speckit.plan; revisar) com bullet final pós-merge listando o que entrou em produção
- [ ] T054 [P] Adicionar entrada no `D:\git\pkd\CHANGELOG.md` (se existir; senão criar) sob "Unreleased" descrevendo feature em PT-BR

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: sem dependências → pode iniciar imediatamente
- **Phase 2 (Foundational)**: depende de Phase 1 → BLOCKS US1/US2/US3
- **Phase 3 (US1) / Phase 4 (US2)**: ambos depende de Phase 2 → podem rodar em paralelo (P1 ambos)
- **Phase 5 (US3)**: depende de **US2** (reusa handler de restore); inicia após Phase 4 completa
- **Phase 6 (Polish)**: depende de US1+US2 (US3 desejável); inicia após Phase 4 mínimo

### User Story Dependencies

- **US1 (P1)** — Backup S3: **independente** (após Foundational). Entrega MVP sozinho — admin já pode gerar ZIP, mas restore continua no caminho legado.
- **US2 (P1)** — Restore cross-backend: **independente** de US1 em execução (após Foundational), porém **complementa** US1 (ZIP gerado por US1 é consumido por US2 em validação manual de quickstart).
- **US3 (P2)** — Restore in-place: **depende de US2** (reusa handler). Após US2 done, US3 adiciona testes + auditoria de cleanup.

### Within Each User Story

- Tests (Phase X.tests) ANTES de implementação (devem falhar antes; passar depois)
- Foundational types (Phase 2) ANTES de qualquer task de story
- Backend (handlers + lógica) ANTES de frontend (`Admin.svelte`)
- Endpoints registrados (`server.go`) ANTES de testes de contract rodarem verde

### Parallel Opportunities

- **Phase 1**: T002, T003, T004 todos [P] após T001 instalar dep
- **Phase 2**: T006, T007, T008, T009 [P] (mesmo arquivo `attachments.go` → na verdade conflitam!) → **revisar**: T006/T007/T008/T009 editam mesmo arquivo, sequenciar. T010 [P] em arquivo distinto.
  - **Correção**: T006, T007, T008 são edições no MESMO `internal/store/attachments.go` — devem ser sequenciais OU agrupadas num único patch. Manter rótulo [P] apenas se feitas como edições atômicas separadas em range distintos (edit tool permite). Remover [P] se incerto.
- **Phase 3 (US1) tests**: T017, T018, T019, T020, T021, T022 todos arquivos distintos → [P]
- **Phase 4 (US2) tests**: T032–T035 arquivos distintos → [P]
- **Phase 5 (US3) tests**: T042–T044 arquivos distintos → [P]
- **Phase 6**: T047, T048, T050, T051, T052, T054 majoritariamente [P]; T053 sequencial (já feito)

### Inter-Story Parallelism

Após Phase 2: US1 implementação + US2 implementação podem ser feitas em paralelo por devs distintos. US1 toca `writer.go`, `sweep.go`; US2 toca `reader.go`; ambos tocam `handlers_admin_storage.go` mas em handlers distintos (`Start`/`GetJob`/`RegenerateURL` vs `RestoreStart`) — coordenar via PRs separados ou edits não-sobrepostos.

---

## Parallel Example: User Story 1 tests

```bash
# Todos podem rodar em paralelo (arquivos distintos):
Task: "T017 Unit test manifest em tests/unit/manifest_test.go"
Task: "T018 Unit test BackupJobManager em tests/unit/jobs_test.go"
Task: "T019 Integration test backup local em tests/integration/backup_local_test.go"
Task: "T020 Integration test backup S3 em tests/integration/backup_s3_test.go"
Task: "T021 Integration test backfill SHA em tests/integration/backup_backfill_sha_test.go"
Task: "T022 Contract test backup endpoints em tests/contract/openapi_test.go"
```

---

## Implementation Strategy

### MVP Slice (US1 apenas)

1. Phase 1 (Setup): T001–T004
2. Phase 2 (Foundational): T005–T016
3. Phase 3 (US1): T017–T031
4. **STOP & VALIDATE**: quickstart cenário 1 com minio local; admin gera ZIP e baixa via URL pré-assinada
5. Deploy `:edge` para UNRAID; valida com bucket S3 real (ou seguir para US2 antes)

### Incremental Delivery

1. MVP (US1) → demo backup; restauração ainda via path legado para backend local
2. + US2 → demo restauração cross-backend; valida quickstart cenário 2
3. + US3 → cobertura de cenários in-place + failure cleanup auditado
4. + Polish → docs, perf check, IAM nota, sweep validado em produção real

### Parallel Team Strategy

Com 2+ devs após Phase 2:

- Dev A: US1 (T017–T031) — foco em writer.go, sweep, handlers backup
- Dev B: US2 (T032–T041) — foco em reader.go, range reader, handler restore
- Convergem em: `handlers_admin_storage.go` (coordenar edits), `Admin.svelte` (coordenar tab)

Dev C (se houver): testes manuais em quickstart cenários + Polish T047/T048/T054 enquanto A/B terminam.

---

## Notes

- [P] tasks = arquivos distintos, sem dependência em task incompleta da mesma fase
- Foundational Phase 2 tem várias edições no MESMO `attachments.go` (T006–T009) — **sequenciar** mesmo se rotuladas [P] no resumo (manter [P] indica "podem ser planejadas independentemente"; execução pode batch num único PR-patch)
- Tests escritos ANTES da implementação correspondente; rodar `go test` para confirmar RED antes de codar GREEN
- Commit após cada checkpoint (após Phase 2; após cada User Story)
- Cleanup de temp objects S3 é CRÍTICO em todos paths de erro (FR-009, SC-005) — auditoria explícita em T045
- Sweep no startup (T024+T025) é **non-blocking goroutine** — não atrasa HTTP listener
- Backend local mantém endpoints legados nesta entrega (T049) — remoção em release futura após validação
- Política de branch: tudo em `main`; cada checkpoint = commit; PR opcional (CLAUDE.md regra)
