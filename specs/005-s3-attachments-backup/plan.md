# Implementation Plan: Backup & Restauração de Arquivos Associados com Backend S3

**Branch**: `main` (política do projeto: sem branches longas) | **Date**: 2026-05-18 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/005-s3-attachments-backup/spec.md`

## Summary

Estender as ações administrativas de backup/restauração de anexos para funcionar quando o backend ativo for **S3**, sem materializar o ZIP no disco da instância de aplicação (EC2). Abordagem:

1. **Backup**: `io.Pipe` conecta `archive/zip.Writer` (encoder) ao `manager.Uploader.Upload` (multipart streaming para S3 `_backup-tmp/<job-id>.zip`). Para cada anexo: `S3.GetObject` → copy para zip writer. Entrada final do ZIP é `manifest.json` (SHA256 → lista de `stored_filename`s + tamanho + MIME). Após upload concluir, gerar URL pré-assinada (TTL 15min) via `s3.PresignClient` e devolver ao admin.
2. **Restore S3 backend**: admin envia ZIP via multipart → servidor faz multipart upload para `_backup-tmp/<job-id>.zip` → lê via Range GETs (wrap em `io.ReaderAt`) → `archive/zip.NewReader` → para cada entrada com SHA256 que bate em `attachments`, faz fan-out `S3.PutObject` por `stored_filename` correspondente.
3. **Restore local backend**: usa temp file local (UNRAID tem disco).
4. **Job tracking**: nova estrutura `BackupJobManager` em memória (single in-flight por backend, FR-015) com endpoint de polling de status/progresso.
5. **Cleanup pós-crash**: sweep no startup remove objetos em `_backup-tmp/` com idade > 24h.
6. **DB**: novo índice em `attachments(content_sha256)`; popular hashes ausentes durante backup (FR-020).

## Technical Context

**Language/Version**: Go 1.25 (backend, CGO disabled); Svelte 5 + Vite (frontend)
**Primary Dependencies**:
- Backend: `github.com/aws/aws-sdk-go-v2/service/s3` v1.101.0 (já presente), `github.com/aws/aws-sdk-go-v2/feature/s3/manager` (**adicionar**), `archive/zip` stdlib, `crypto/sha256` stdlib, `github.com/google/uuid` v1.6.0 (já presente), `github.com/go-chi/chi/v5` v5.2.5 (já presente)
- Frontend: nenhum novo
**Storage**: SQLite (modernc.org/sqlite); adiciona índice `idx_attachments_content_sha256`; backend S3 ganha prefixo reservado `_backup-tmp/`
**Testing**: `go test ./...`; novos testes em `tests/unit/zip_manifest_test.go`, `tests/integration/backup_restore_s3_test.go` (mock S3 via interface), `tests/contract/openapi_test.go` (estender com novos endpoints)
**Target Platform**: Linux container (Docker, CGO off). Runtime: EC2 (prod, backend S3) e UNRAID (dev, backend local)
**Project Type**: Web service (backend Go + frontend Svelte). Estrutura existente `backend/ frontend/ tests/` será reusada
**Performance Goals**: backup de 10 GB (FR/SC-004) conclui sem aumento de footprint de memória além de buffers de streaming (~ 5 MB part size do multipart S3); cada entrada processada em O(tamanho do arquivo) sem materialização integral
**Constraints**:
- ZERO escrita do conteúdo agregado do ZIP em disco local da aplicação (FR-002, SC-001)
- Streaming end-to-end: entrada do S3 → zip writer → pipe → S3 multipart upload, tudo via `io.Reader`/`io.Writer`
- Memória de heap < 50 MB adicional durante operação (limite operacional EC2 t3.micro / t3.small)
- Uma operação por backend ativo (FR-015)
**Scale/Scope**: até 10 GB total agregado, até ~10K anexos típico, arquivo individual sem limite artificial (ZIP64, FR-018)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Arquivo `.specify/memory/constitution.md` contém apenas placeholders de template (não preenchido). Usar como guia normativo:

1. **CLAUDE.md global**: `simplicidade > complexidade`; `evitar vulnerabilidades de segurança desde início`; `usar best practices`; `só branch main`.
2. **CLAUDE.md do projeto**: Go 1.25 + Svelte 5; SQLite; estrutura `backend/ frontend/ tests/`.

### Gate evaluation

| Gate | Status | Notas |
|---|---|---|
| Simplicidade (CLAUDE.md global) | ✅ | Reusa Backend interface existente; adiciona `manager` (oficial AWS) ao invés de custom multipart; sem nova lib de jobs (mutex + map) |
| Segurança (CLAUDE.md global) | ✅ | URLs pré-assinadas TTL 15min; sem credenciais no log; auth admin obrigatório; sweep de temp objects; ZIP validado por SHA256 |
| Tamanho do Docker | ✅ | `manager` é parte do aws-sdk-go-v2 já incluído; sem nova dependência runtime de imagem; ZIP64 via stdlib |
| Branch policy | ✅ | Spec/plan vivem em `specs/005-...` na branch `main`; nenhuma branch longa criada |
| Backwards-compat | ✅ | Local backend continua funcionando; endpoints novos coexistem com antigos durante transição |

Sem violações. Sem entradas em Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/005-s3-attachments-backup/
├── plan.md              # This file
├── spec.md              # Feature spec (criada em /speckit.specify)
├── research.md          # Phase 0 output (este comando)
├── data-model.md        # Phase 1 output (este comando)
├── quickstart.md        # Phase 1 output (este comando)
├── contracts/
│   └── api.yaml         # OpenAPI delta para novos endpoints
├── checklists/
│   └── requirements.md  # Criado em /speckit.specify
└── tasks.md             # /speckit.tasks output (próximo comando)
```

### Source Code (repository root)

```text
backend (Go)/
├── cmd/                                # main package(s) — sem mudança nesta feature
├── internal/
│   ├── server/
│   │   ├── handlers_admin_storage.go   # ESTENDER: novos handlers backup_start/restore_start/job_status
│   │   ├── jobs.go                     # NOVO: BackupJobManager (in-memory, mutex, single in-flight per backend)
│   │   ├── server.go                   # ESTENDER: novas rotas + chamada de startup sweep
│   │   └── (handlers existentes preservados para local backend até frontend migrar)
│   ├── storage/
│   │   ├── s3.go                       # ESTENDER: PresignGet(key, ttl); GetRange(key, off, len); MultipartUploadFromReader(key, r)
│   │   └── storage.go                  # ESTENDER: opcional interface S3Capable (type assertion)
│   ├── store/
│   │   ├── attachments.go              # ESTENDER: EnumerateForBackup() iterator; LookupBySHA256(); BackfillSHA256()
│   │   └── migrate.go                  # ESTENDER: nova migração — índice content_sha256
│   └── backup/                         # NOVO: pacote isolado para lógica de ZIP+manifesto
│       ├── manifest.go                 # estruturas Manifest, ManifestEntry; encode/decode JSON
│       ├── writer.go                   # StreamingBackup(ctx, src Backend, attachments []Att, sink io.Writer) → composição zip
│       ├── reader.go                   # StreamingRestore(ctx, zipSrc io.ReaderAt, size int64, dst Backend, lookup Lookup) → fan-out
│       └── sweep.go                    # SweepStaleTempObjects(ctx, s3Backend, prefix, maxAge) executado no startup

frontend (Svelte 5)/
└── src/lib/components/
    └── Admin.svelte                    # ESTENDER: tab Storage — UI de job (botão start, polling de status, link de download)

tests/
├── unit/
│   ├── manifest_test.go                # NOVO: encode/decode + edge cases
│   └── jobs_test.go                    # NOVO: concorrência, single in-flight
├── integration/
│   ├── backup_local_test.go            # NOVO: roundtrip local→ZIP→local
│   ├── backup_s3_mock_test.go          # NOVO: roundtrip com fake S3 (interface ou minio em CI)
│   └── backup_restore_cross_test.go    # NOVO: S3→ZIP→local e vice-versa (cross-backend)
└── contract/
    └── openapi_test.go                 # ESTENDER: validar novos endpoints contra openapi.yaml
```

**Structure Decision**: Mantém estrutura `backend/ frontend/ tests/` já existente. Novo pacote `internal/backup/` isola lógica ZIP+manifesto+sweep do servidor HTTP — facilita testar sem subir chi router. Lógica de job tracking em `internal/server/jobs.go` por proximidade dos handlers.

## Complexity Tracking

> Vazio — sem violações de gates.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none) | (none) | (none) |
