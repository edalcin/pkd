# Changelog

Todas as mudanças notáveis nesta aplicação.

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/).

## [Unreleased]

### Adicionado

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
