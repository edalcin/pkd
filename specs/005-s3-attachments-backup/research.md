# Phase 0 Research — S3 Streaming Backup/Restore

**Date**: 2026-05-18
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)

Sem entradas `NEEDS CLARIFICATION` no Technical Context. Esta sessão consolida decisões técnicas chave derivadas do spec + clarifications.

---

## R1. Streaming ZIP → S3 sem disco local

**Decision**: Compor ZIP via `archive/zip.NewWriter(pw)` onde `pw` é o lado *writer* de um `io.Pipe()`. O lado *reader* `pr` é consumido por `manager.Uploader.Upload(ctx, &s3.PutObjectInput{Bucket, Key, Body: pr})` rodando em goroutine. Esse padrão é o canônico do aws-sdk-go-v2 para upload de tamanho desconhecido com streaming.

**Rationale**:
- `manager.Uploader` faz multipart automaticamente, usando part size default de 5 MiB (mínimo do S3) e até 10K partes — cobre objetos até ~50 GB sem ajuste.
- `io.Pipe` é stdlib, sem buffer adicional; back-pressure natural (goroutine writer bloqueia quando uploader não consome).
- Compatível com ZIP64: `archive/zip` ativa ZIP64 automaticamente quando entrada ou central directory excede 4 GB.

**Alternatives considered**:
- **Buffer em memória completo**: viola constraint de footprint < 50 MB para volumes de 10 GB. Rejeitado.
- **Temp file local**: viola FR-002/SC-001 (zero conteúdo agregado em disco EC2). Rejeitado.
- **Multipart manual** (sem `manager`): reinventa retry, abort em erro, e cálculo de part size. Manager já é parte do aws-sdk-go-v2; sem custo de dependência adicional além de importar subpath.
- **S3 Multipart com `UploadPart` direto + ZIP que materializa entradas individuais**: complica composição streaming do ZIP. Rejeitado por complexidade.

**Code shape (referência, não implementação)**:
```go
pr, pw := io.Pipe()
uploader := manager.NewUploader(s3Client)
errCh := make(chan error, 1)
go func() {
    _, err := uploader.Upload(ctx, &s3.PutObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(tempKey),
        Body:   pr,
    })
    errCh <- err
}()
zw := zip.NewWriter(pw)
// ... write entries ...
if err := zw.Close(); err != nil { pw.CloseWithError(err); return err }
if err := pw.Close(); err != nil { return err }
if err := <-errCh; err != nil { return err }
```

---

## R2. Pre-signed URL para download (TTL 15 min)

**Decision**: Usar `s3.NewPresignClient(client)` + `PresignGetObject(ctx, &s3.GetObjectInput{Bucket, Key}, s3.WithPresignExpires(15*time.Minute))`. URL é devolvida ao frontend no payload de status do job; frontend faz `window.location = url` (ou anchor com `download` attr) para iniciar download direto S3 → navegador.

**Rationale**:
- Não passa pelo backend; satisfaz constraint de não-materialização no disco EC2 nem no socket TCP da aplicação.
- TTL curto (15min) reduz risco de vazamento em logs/proxies.
- Re-gerável sob demanda chamando handler novamente para o mesmo job (enquanto temp object existe).

**Alternatives considered**:
- **Proxy via aplicação** (`s3.GetObject` → ResponseWriter): aceitável para arquivos pequenos, mas força o tráfego pelo EC2, consome banda e CPU. Para um ZIP de 10 GB inviabiliza. Rejeitado.
- **TTL longo + revogação manual**: S3 presigned URLs não são revogáveis individualmente (apenas pela rotação da credencial). Rejeitado.

---

## R3. Restore — ZIP precisa de random access

**Problema**: ZIP central directory fica no FIM do arquivo. `zip.NewReader(r io.ReaderAt, size int64)` exige random access. Multipart upload incoming → não é seekable.

**Decision**:
- Backend ativo = **S3**: stream-upload da requisição HTTP para `_backup-tmp/<job-id>-restore.zip` (mesmo padrão de R1). Depois, ler entradas via Range GETs (`s3.GetObject` com `Range: bytes=off-end`) wrap em adapter `io.ReaderAt`. Passar para `zip.NewReader`. Deletar temp object ao fim.
- Backend ativo = **Local**: gravar requisição em temp file local (`os.CreateTemp` em diretório do backend local, **não** no FS da aplicação se distinto — UNRAID tem espaço). Usar `os.File` como `io.ReaderAt`.

**Rationale**:
- Mantém constraint de zero conteúdo agregado no disco do EC2 (S3 case).
- Local backend não tem essa restrição (FR diz "instância EC2"); usar disco local é simples e barato.
- Range GETs no S3 são baratos e suportados nativamente; latência ~10-50ms por request é aceitável para central directory + descompressão entrada-por-entrada.

**Alternatives considered**:
- **Forçar ZIP estendido com manifesto no INÍCIO** (custom format): quebra compat com ferramentas ZIP padrão. Rejeitado.
- **Tee + buffer em memória + lookup do central directory**: para ZIPs de muitos GB, inviável em memória. Rejeitado.

---

## R4. Manifesto: formato e localização no ZIP

**Decision**: Arquivo `manifest.json` é a **última entrada** escrita no ZIP. JSON Schema:

```json
{
  "version": 1,
  "created_at": "2026-05-18T12:34:56Z",
  "source_environment": {
    "backend_kind": "s3",
    "bucket": "pkd-prod-bucket",
    "prefix": "attachments/"
  },
  "entries": [
    {
      "sha256": "abc123...",
      "size_bytes": 4096,
      "mime_type": "application/pdf",
      "stored_filenames": ["att-001.pdf", "att-007.pdf"]
    }
  ]
}
```

**Rationale**:
- JSON: humano-legível, debugável; suportado pela stdlib; sem overhead de geração binária.
- Última entrada: writer pode atrasar escrita até processar todos os anexos (necessário se computar SHA256 inline durante backup — FR-020).
- `stored_filenames` é array (não escalar) para suportar dedup: múltiplas linhas `attachments` podem compartilhar mesmo SHA256 (FR-019, FR-021).
- `source_environment` é informativo (auditoria); restore não depende dele.

**Alternatives considered**:
- **CSV**: menos verboso mas pior para nested arrays e evolução de schema. Rejeitado.
- **Manifesto no INÍCIO**: forçaria duas passadas sobre os anexos (uma para enumerar/hashear, outra para escrever no ZIP). Rejeitado por dobrar tempo de backup.
- **SQLite embedded como manifesto**: overkill. Rejeitado.

---

## R5. Backfill de `content_sha256` ausentes (FR-020)

**Decision**: Durante backup, para cada linha de `attachments` com `content_sha256 IS NULL`, computar SHA256 ao ler o arquivo da origem (uma passada de leitura serve tanto para hash quanto para zip writer via `io.TeeReader`). Persistir o hash via `UPDATE attachments SET content_sha256 = ? WHERE id = ?` antes de escrever a entrada no ZIP.

**Rationale**:
- Aproveita a leitura única que já é feita; sem custo extra de I/O.
- Persiste o valor para evitar recomputação em backups futuros.
- Restore depende de SHA256 estar populado nas linhas da base de destino — backup é o trigger natural para garantir isso na origem.

**Alternatives considered**:
- **Migração standalone** que varre todos os arquivos no startup: custosa, bloqueante, e desnecessária se backup já cobre.
- **Computar sem persistir**: força re-hashing em cada backup. Rejeitado.

---

## R6. Job tracking — in-memory, single in-flight por backend

**Decision**: `internal/server/jobs.go` exporta `BackupJobManager`:

```go
type Job struct {
    ID            string    // uuid
    Kind          string    // "backup" | "restore"
    BackendKind   string    // "local" | "s3"
    State         string    // "running" | "succeeded" | "failed"
    Processed     int64
    Total         int64
    StartedAt     time.Time
    EndedAt       time.Time
    ErrorMessage  string
    DownloadURL   string    // presigned URL (backup) ou empty
    URLExpiresAt  time.Time
    SkippedEntries []string // SHA256s ignorados em restore (FR-017)
}
type BackupJobManager struct {
    mu       sync.Mutex
    active   map[string]*Job // key = backendKind; max 1 active per backend
    history  map[string]*Job // key = job.ID; LRU bounded
}
```

`StartJob(kind, backendKind)` retorna erro se já há job ativo para esse backend (FR-015). `GetJob(id)` para polling pelo frontend.

**Rationale**:
- Aplicação é single-binary; mutex em memória é suficiente.
- Sem persistência de jobs porque crash significa job perdido (sweep limpa temp objects no startup, FR-009b).
- LRU bounded (~50 entries) evita vazamento de memória para histórico.

**Alternatives considered**:
- **Persistir jobs no SQLite**: adiciona schema + risco de corrupção em crash; histórico tem valor limitado (admin já vê erro no log). Rejeitado.
- **Queue distribuída** (NATS, Redis): viola simplicidade (CLAUDE.md). Rejeitado.

---

## R7. Sweep de órfãos no startup

**Decision**: `backup.SweepStaleTempObjects(ctx, s3Backend, "_backup-tmp/", 24*time.Hour)` chamado em `server.New()` quando backend ativo for S3, **não bloqueante** (goroutine). Lista objetos no prefixo via `s3.ListObjectsV2` paginator, filtra por `LastModified < now - 24h`, deleta em batch via `s3.DeleteObjects` (max 1000 por chamada).

**Rationale**:
- Não bloqueia startup (admin pode usar app antes do sweep terminar).
- 24h é folga suficiente para qualquer backup legítimo concluir (tipicamente minutos para 10 GB).
- Operação read-then-delete é idempotente.

**Alternatives considered**:
- **S3 Lifecycle Policy** no bucket: requer configuração operacional fora do código, frágil em diferentes ambientes. Rejeitado.
- **Cron interno**: já temos throttle sweepLoop como precedente; mas startup-only é mais simples e suficiente. Aceito como complemento futuro, fora do escopo.

---

## R8. Concorrência e backpressure durante streaming

**Decision**: Single goroutine por job. Writer goroutine alimenta pipe sequencialmente (anexo por anexo). Uploader goroutine consome pipe. Sem fan-out paralelo no v1.

**Rationale**:
- Anexos lidos sequencialmente do S3 evitam saturar conexão e/ou rate-limit do bucket.
- Paralelismo agregaria complexidade (sincronização do zip writer, que NÃO é thread-safe) sem ganho claro para volumes alvo (10 GB).
- Para volumes maiores futuramente, paralelizar leitura de anexos em pool + canal serializado para o zip writer é evolução natural.

**Alternatives considered**:
- **Pool de N workers lendo do S3 em paralelo**: requer canal serializado para zip writer; ganho marginal e risco de exhaustar fd/conexões. Rejeitado para v1.

---

## R9. Testabilidade do backend S3

**Decision**: Definir interface mínima `S3API` em `internal/storage/s3.go` que cubra os métodos usados (`GetObject`, `PutObject`, `ListObjectsV2`, `DeleteObjects`, presign). Mock em testes via implementação manual ou via `github.com/aws/aws-sdk-go-v2/service/s3/s3iface`-style stubs locais.

**Rationale**:
- Permite testes unitários sem rede.
- Para teste de integração end-to-end, usar **minio** em container CI (já comum em projetos similares); decisão sobre minio em CI fica para `/speckit.tasks` (operacional).

**Alternatives considered**:
- **localstack**: imagem grande, lento. Rejeitado para CI rápida.
- **Apenas testes de integração contra S3 real**: requer credenciais em CI, lento e custoso. Rejeitado.

---

## Resumo

Nenhum unknown técnico bloqueante. Todas decisões alinham com clarifications da sessão 2026-05-18. Pronto para Phase 1 (data-model, contracts, quickstart).
