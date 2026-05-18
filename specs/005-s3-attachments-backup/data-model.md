# Phase 1 Data Model

**Date**: 2026-05-18
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)

## Tabelas SQLite (alterações)

### `attachments` — alteração mínima

Tabela existente. Coluna `content_sha256 TEXT` já existe (nullable, migração `internal/store/migrate.go:99`). Esta feature adiciona apenas índice.

```sql
-- Nova migração: idx_attachments_content_sha256
CREATE INDEX IF NOT EXISTS idx_attachments_content_sha256
    ON attachments(content_sha256);
```

**Justificativa**: restore localiza linhas por SHA256 (FR-021). Sem índice, lookup é O(N) por entrada do ZIP — inviável com 10K anexos × N entradas.

Não há ALTER TABLE; coluna persiste como `TEXT` nullable. Backup popula valores faltantes (FR-020).

### `backup_jobs` — NÃO criar

Decisão (R6 da research): jobs vivem em memória. Histórico bounded LRU. Sem persistência.

---

## Estruturas Go (in-memory)

### `backup.Manifest` (pacote `internal/backup`)

```go
type Manifest struct {
    Version           int             `json:"version"`             // = 1
    CreatedAt         time.Time       `json:"created_at"`
    SourceEnvironment EnvDescriptor   `json:"source_environment"`
    Entries           []ManifestEntry `json:"entries"`
}

type EnvDescriptor struct {
    BackendKind string `json:"backend_kind"` // "local" | "s3"
    Bucket      string `json:"bucket,omitempty"`
    Prefix      string `json:"prefix,omitempty"`
}

type ManifestEntry struct {
    SHA256          string   `json:"sha256"`           // hex lowercase, 64 chars
    SizeBytes       int64    `json:"size_bytes"`
    MimeType        string   `json:"mime_type,omitempty"`
    StoredFilenames []string `json:"stored_filenames"` // ≥ 1; ordem irrelevante
}
```

**Validações**:
- `Version == 1` (reject outros valores no restore — futuro pode evoluir)
- `SHA256` matches `^[0-9a-f]{64}$`
- `SizeBytes >= 0`
- `StoredFilenames` não vazio
- Backup gera `StoredFilenames` agrupando por SHA256 (uma entrada do ZIP por hash único)
- Restore lê o array e faz fan-out (uma escrita no backend por filename)

### `server.Job` (pacote `internal/server`)

```go
type Job struct {
    ID             string    // uuid v4
    Kind           string    // "backup" | "restore"
    BackendKind    string    // "local" | "s3"
    State          string    // "running" | "succeeded" | "failed"
    Processed      int64     // entradas processadas (anexos para backup; arquivos restaurados para restore)
    Total          int64     // 0 se desconhecido (e.g. início de restore antes de ler manifest)
    StartedAt      time.Time
    EndedAt        time.Time // zero se ainda rodando
    AdminUserID    int64     // identidade que iniciou (FR-011)
    ErrorMessage   string    // populado se State == "failed"
    DownloadURL    string    // presigned URL (backup S3) — vazio para outros
    URLExpiresAt   time.Time // zero se sem download URL
    SkippedEntries []SkippedEntry // restore: entradas ignoradas (FR-017)
    SizeBytes      int64     // tamanho final do ZIP (backup) ou da operação (restore)
}

type SkippedEntry struct {
    SHA256    string
    SizeBytes int64
    Reason    string // "no matching attachment row"
}
```

**Estado lifecycle**:
```
running → succeeded
running → failed
```

Sem transição reversa. Sem retomada. Crash da app = job perdido (sweep limpa temp objects).

### `server.BackupJobManager`

```go
type BackupJobManager struct {
    mu          sync.Mutex
    activeByBackend map[string]*Job // key = backendKind ("local" | "s3"); valor = job em "running"
    history     *lru.Cache[string, *Job] // key = job.ID; cap 50
}

func (m *BackupJobManager) Start(kind, backendKind string, adminID int64) (*Job, error)
// erro: ErrJobInFlight se já existe job ativo para backendKind (FR-015)

func (m *BackupJobManager) Get(id string) (*Job, bool)

func (m *BackupJobManager) finishLocked(j *Job, state string, errMsg string) // chamado pela goroutine ao terminar
```

**Bounded history**: usar `github.com/hashicorp/golang-lru/v2` (já no ecossistema Go) ou implementar trivial LRU manual (preferido para evitar dep nova; cap 50 é simples).

---

## Storage layer (interface delta)

### `internal/storage/s3.go` — métodos novos

```go
// Existentes: Put, Get, Delete, List

// Novo: range GET para suportar zip.NewReader(io.ReaderAt) sobre objeto S3
func (b *S3Backend) GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error)

// Novo: presigned GET URL com TTL
func (b *S3Backend) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)

// Novo: streaming upload (delegado ao manager.Uploader)
func (b *S3Backend) UploadFromReader(ctx context.Context, key string, body io.Reader, contentType string) error

// Novo: list temp objects para sweep
func (b *S3Backend) ListWithMetadata(ctx context.Context, prefix string) ([]ObjectMeta, error)

// Novo: batch delete (até 1000 por chamada)
func (b *S3Backend) DeleteMany(ctx context.Context, keys []string) error

type ObjectMeta struct {
    Key          string
    SizeBytes    int64
    LastModified time.Time
}
```

### `internal/storage/storage.go` — type assertion opcional

```go
// S3Capable expõe operações S3-específicas. Local backend NÃO implementa.
// Handlers fazem type-assertion para detectar capacidades.
type S3Capable interface {
    GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error)
    PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
    UploadFromReader(ctx context.Context, key string, body io.Reader, contentType string) error
    ListWithMetadata(ctx context.Context, prefix string) ([]ObjectMeta, error)
    DeleteMany(ctx context.Context, keys []string) error
}
```

Backend interface **não muda**. `S3Capable` é separado; checagem em handler:
```go
if cap, ok := activeBackend.(storage.S3Capable); ok {
    // S3 path: streaming via cap
} else {
    // Local path: temp file + zip writer to response
}
```

---

## Store layer (delta)

### `internal/store/attachments.go` — funções novas

```go
// EnumerateForBackup retorna iterator/canal de anexos a serem incluídos.
// Inclui apenas anexos referenciados (linhas reais da tabela).
type AttachmentForBackup struct {
    ID              int64
    StoredFilename  string
    MimeType        string
    SizeBytes       int64
    StorageLocation string  // "local" | "s3"
    ContentSHA256   string  // pode ser "" se ainda não computado
}

func (s *AttachmentStore) EnumerateForBackup(ctx context.Context) ([]AttachmentForBackup, error)
// Para 10K linhas, batch via SELECT direto é viável. Para escala maior, refatorar para iterator.

// LookupBySHA256 retorna todas linhas que apontam para mesmo conteúdo.
func (s *AttachmentStore) LookupBySHA256(ctx context.Context, sha256 string) ([]AttachmentRef, error)

type AttachmentRef struct {
    ID              int64
    StoredFilename  string
    StorageLocation string
}

// BackfillSHA256 atualiza a linha após backup computar o hash.
func (s *AttachmentStore) BackfillSHA256(ctx context.Context, attachmentID int64, sha256 string) error
```

---

## Convenções de nomenclatura S3

| Item | Padrão | Exemplo |
|---|---|---|
| Anexo "real" | `<config.S3.Prefix>/<stored_filename>` | `attachments/att-001.pdf` |
| ZIP temporário backup | `_backup-tmp/<job-id>.zip` | `_backup-tmp/abc-123.zip` |
| ZIP temporário restore | `_backup-tmp/<job-id>-restore.zip` | `_backup-tmp/xyz-789-restore.zip` |

**Restrição**: prefixo `_backup-tmp/` é reservado. Sweep e handlers de backup/restore podem deletar livremente. Validador defensivo em `AttachmentStore.CreateFile` deve rejeitar `stored_filename` que comece com `_backup-tmp/` (defesa em profundidade).

---

## Fluxo de dados — Backup S3

```
HTTP POST /api/admin/storage/backup-start
    │
    ▼
BackupJobManager.Start("backup", "s3", adminID)
    │ (acquire lock; create Job; release lock)
    ▼
goroutine:
    EnumerateForBackup() → []AttachmentForBackup
    │
    ▼
    pr, pw := io.Pipe()
    │
    ├─ goroutine A: zip.NewWriter(pw)
    │     for each attachment:
    │       src := S3.Get(stored_filename)
    │       hasher := sha256.New()
    │       tee := io.TeeReader(src, hasher)
    │       zw.CreateHeader(name=sha256_or_pending) → io.Copy(zwEntry, tee)
    │       if att.ContentSHA256 == "":
    │         BackfillSHA256(att.ID, hex(hasher))
    │       Job.Processed++ (atomic)
    │     write manifest.json as last entry
    │     zw.Close(); pw.Close()
    │
    └─ goroutine B: manager.Uploader.Upload(Body: pr) → S3 _backup-tmp/<id>.zip
    │
    ▼ both join
    presignedURL := S3.PresignGet(tempKey, 15min)
    Job.DownloadURL = presignedURL; Job.URLExpiresAt = now+15min
    Job.State = "succeeded"; Job.EndedAt = now
```

Nota: nome de cada entrada no ZIP precisa do SHA256, que pode ser desconhecido até terminar de ler. Solução: usar **agrupamento de duas passadas leves**:
1. Primeira passada: enumerar + para linhas SEM sha, baixar do source, computar hash, BackfillSHA256, **descartar conteúdo**. Para linhas COM sha, manter como está.
2. Segunda passada (a que streamea): para cada SHA único, baixar uma vez do source, copiar para zip entry nomeada `<sha256>`.

Custo extra: para anexos sem hash, **dois GETs** do S3 (um para hashear, um para zippar). Aceitável pois é one-time (backfill persiste). Para anexos já hasheados (caso comum em produção S3), apenas um GET.

**Alternativa avaliada**: usar nome temporário no header e re-escrever após — impossível com `archive/zip` (headers são imutáveis após `CreateHeader`). Rejeitada.

---

## Fluxo de dados — Restore (cross-backend ou in-place)

```
HTTP POST /api/admin/storage/restore-start (multipart upload do ZIP)
    │
    ▼
BackupJobManager.Start("restore", backendKind, adminID)
    │
    ▼
goroutine:
    if S3Capable(activeBackend):
        S3.UploadFromReader(_backup-tmp/<id>-restore.zip, requestBody)
        zipSrc := s3RangeReaderAt(s3, key)
        size  := head_object.size
    else:
        f := os.CreateTemp(localBackendDir, "restore-*.zip")
        io.Copy(f, requestBody)
        zipSrc := f; size := stat.Size
    │
    ▼
    zr := zip.NewReader(zipSrc, size)
    manifest := decode(zr.File["manifest.json"])
    Job.Total = sum(len(e.StoredFilenames) for e in manifest.Entries)
    │
    ▼
    for each entry in manifest.Entries:
        refs := LookupBySHA256(entry.SHA256)
        if len(refs) == 0:
            Job.SkippedEntries = append(..., SkippedEntry{SHA256, Size, "no matching attachment row"})
            continue
        zipFile := zr.File[entry.SHA256]
        rc := zipFile.Open()
        // Verify integrity inline
        hasher := sha256.New()
        // Buffer or stream? Need to write to multiple destinations potentially.
        // Strategy: read once into memory if size < threshold (e.g. 50MB), else multi-pass GETs from ZIP (cheap, in-process).
        content := io.ReadAll(rc)
        if hex(sha256(content)) != entry.SHA256:
            log error per file; continue (do not write)
        for each ref in refs:
            activeBackend.Put(ref.StoredFilename, bytes.NewReader(content), len(content), entry.MimeType)
            Job.Processed++
    │
    ▼
    cleanup temp ZIP (S3 Delete or os.Remove)
    Job.State = "succeeded"
```

---

## Estados / transições

Já cobertos em `Job.State`. Sem máquinas de estado adicionais.
