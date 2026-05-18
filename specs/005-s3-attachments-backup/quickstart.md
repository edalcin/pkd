# Quickstart — Backup & Restauração com Backend S3

**Audience**: desenvolvedor implementando esta feature; operador validando comportamento em ambientes alvo.

## Pré-requisitos

- Repo PKD em `D:\git\pkd` (Windows) ou equivalente
- Go 1.25, Node 20+ (para frontend), `pwsh` no PATH
- Para testar S3 localmente: `docker run -p 9000:9000 -p 9001:9001 minio/minio server /data --console-address ":9001"`
- Bucket de teste criado: `mc alias set local http://localhost:9000 minioadmin minioadmin && mc mb local/pkd-test`

## Configuração de ambiente

Variáveis (já existentes; nenhuma nova nesta feature):
```
PKD_S3_BUCKET=pkd-test
PKD_S3_REGION=us-east-1
PKD_S3_PREFIX=attachments/
PKD_S3_ACCESS_KEY_ID=minioadmin
PKD_S3_SECRET_ACCESS_KEY=minioadmin
PKD_STORAGE_BACKEND=s3              # ou "local"
PKD_S3_ENDPOINT=http://localhost:9000  # se já existir no Load(); senão usar AWS default
```

## Smoke tests

### 1. Backup com backend S3 (cenário P1)

```bash
# 1. subir aplicação com backend s3
go build -o bin/pkd ./cmd/pkd && PKD_STORAGE_BACKEND=s3 ./bin/pkd

# 2. autenticar como admin (via UI ou curl com cookie de sessão salvo)
# 3. iniciar backup
curl -X POST http://localhost:8080/api/admin/storage/backup-start \
     -H "Cookie: pkd_session=<token>" \
     -i
# Esperado: 202 Accepted, { "job_id": "..." }

# 4. polling de status
JOB=<job_id_da_resposta>
curl http://localhost:8080/api/admin/storage/jobs/$JOB \
     -H "Cookie: pkd_session=<token>"
# Esperado: state="running" → "succeeded", download_url populado

# 5. baixar o ZIP
curl -L "$(curl -s .../jobs/$JOB -H "Cookie: ..." | jq -r .download_url)" -o backup.zip

# 6. verificar conteúdo
unzip -l backup.zip
# Esperado: 1 entrada por SHA256 único + manifest.json
```

**Checklist de validação**:
- [ ] Disco da máquina rodando `./bin/pkd` NÃO contém o ZIP em nenhum momento (verificar `du -sh /tmp` antes/durante/depois)
- [ ] Bucket `pkd-test` contém objeto em `_backup-tmp/<job-id>.zip` durante operação
- [ ] Após terminar e baixar, o objeto temp continua existindo até expirar 24h OU até sweep no próximo startup
- [ ] `manifest.json` no ZIP tem versão 1 e lista todos os anexos referenciados

### 2. Restauração cross-backend S3 → Local (cenário P1)

```bash
# 1. ter backup.zip gerado no cenário 1

# 2. subir SEGUNDA instância com backend local (porta diferente)
PKD_STORAGE_BACKEND=local PKD_DATA_DIR=/tmp/pkd-dev PKD_HTTP_ADDR=:8081 ./bin/pkd

# 3. importar backup do DB para a instância dev (operação separada, já existente)
#    (necessário para que a tabela attachments exista com mesmas linhas)

# 4. enviar ZIP
curl -X POST http://localhost:8081/api/admin/storage/restore-start \
     -H "Cookie: pkd_session=<token_dev>" \
     -F "zip=@backup.zip" \
     -F "on_conflict=overwrite" \
     -i
# Esperado: 202 Accepted, { "job_id": "..." }

# 5. polling
curl http://localhost:8081/api/admin/storage/jobs/$JOB2 -H "Cookie: ..."
# Esperado: state="succeeded", processed == total, skipped_entries vazio (se DBs sincronizadas)

# 6. abrir um documento que tenha anexo na UI — deve carregar normalmente
```

**Checklist**:
- [ ] Diretório local `/tmp/pkd-dev/attachments/` contém os arquivos do backup
- [ ] Hash de cada arquivo restaurado bate com `content_sha256` na base dev
- [ ] Skipped entries são reportadas com SHA256 (apenas para entradas sem linha correspondente)

### 3. Restauração in-place S3 → S3 (cenário P2)

```bash
# 1. remover manualmente alguns objetos do bucket S3 (simular perda)
mc rm local/pkd-test/attachments/att-abc.pdf

# 2. enviar ZIP de backup anterior para a MESMA instância
curl -X POST http://localhost:8080/api/admin/storage/restore-start ... -F zip=@backup.zip
```

**Checklist**:
- [ ] Objetos voltam ao bucket
- [ ] Disco da máquina rodando a app NÃO recebe o ZIP em nenhum momento
- [ ] Temp object `_backup-tmp/<job-id>-restore.zip` é removido ao final

## Testes automatizados

```bash
# unit
go test ./internal/backup/... ./internal/server/... -timeout 60s

# integration (requer minio rodando)
PKD_TEST_S3_ENDPOINT=http://localhost:9000 \
PKD_TEST_S3_BUCKET=pkd-test \
go test -tags=integration ./tests/integration/... -timeout 300s

# contract (validar OpenAPI)
go test ./tests/contract/...
```

## Cenários de falha (validação de SC-005)

| Cenário | Como simular | Expectativa |
|---|---|---|
| Crash durante backup | `kill -9` da app no meio de upload | Próximo startup remove `_backup-tmp/<job-id>.zip` se idade > 24h. Para teste imediato, ajustar `maxAge` para `0` via flag interno. |
| Credencial S3 revogada | rotacionar `PKD_S3_SECRET_ACCESS_KEY` para valor inválido durante upload | Job termina em "failed" com mensagem clara; temp object é tentado limpar (best-effort) |
| ZIP corrompido | `truncate -s 100 backup.zip; curl -F zip=@backup.zip ...` | 400 Bad Request com mensagem "manifest missing or unreadable" |
| Concorrência | iniciar 2 backups simultaneamente | Segundo: 409 Conflict "operation already in progress for backend" |

## Operacional — IAM mínimo para EC2/role

```json
{
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject", "s3:PutObject", "s3:DeleteObject",
        "s3:ListBucket", "s3:DeleteObjects"
      ],
      "Resource": [
        "arn:aws:s3:::pkd-prod-bucket",
        "arn:aws:s3:::pkd-prod-bucket/*"
      ]
    }
  ]
}
```

`s3:DeleteObjects` (batch) é necessário para sweep eficiente. `PresignGetObject` é client-side (não requer permissão IAM além de `s3:GetObject` que o destinatário usa via URL).

## Pontos de saída

- Lógica pura em `internal/backup/` (testável sem rede)
- HTTP em `internal/server/handlers_admin_storage.go` (testável com `httptest`)
- S3-specific em `internal/storage/s3.go` (mock via interface `S3API`)
- UI em `frontend/src/lib/components/Admin.svelte` (testar manual em dev server `npm run dev`)
