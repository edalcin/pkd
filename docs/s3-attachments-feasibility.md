# Relatório de Viabilidade — Armazenamento de Anexos em Amazon S3

**Status:** Análise de viabilidade com decisões confirmadas (entrada para spec formal)
**Data:** 2026-05-02 (revisão 3 — incorpora respostas às perguntas críticas)
**Autor:** Claude (análise técnica), Eduardo Dalcin (proposta e decisões)
**Escopo:** Avaliar a viabilidade e desenhar a abordagem para mover os anexos do PKD para S3, considerando produção em EC2 (com EBS provisionado) e dev/homologação em UNRAID (filesystem local).

---

## 1. Resumo executivo

A funcionalidade é **viável, recomendada, e estrategicamente correta** — mesmo que financeiramente o ganho imediato seja modesto (volume atual: 8.7MB / 94 arquivos). O valor real está em três eixos:

1. **Future-proofing:** o crescimento esperado dos anexos não vai mais consumir EBS provisionado. S3 escala automaticamente sem intervenção.
2. **Separação de responsabilidades:** EC2 cuida de computação, S3 cuida de storage. Cada um pode crescer/reduzir/ser substituído independentemente.
3. **Postura de segurança superior:** IAM Role da EC2 elimina o conceito de "credenciais armazenadas em algum lugar" — não há nada para vazar em produção.

**Esforço estimado:** ~3 a 4 dias para v1 funcional. O volume mínimo elimina toda a complexidade de migração resumível e operações de longa duração.

**Decisões confirmadas (referência rápida):**

| Decisão | Escolha |
|---|---|
| Storage class | S3 Intelligent-Tiering |
| Versioning | Habilitado, com lifecycle "remover versões >90 dias" |
| Buckets | Separados: `pkd-prod-attachments`, `pkd-dev-attachments` |
| Auth em produção (EC2) | IAM Role do Instance Profile (sem credenciais em env var) |
| Auth em dev (UNRAID) | Access Key + Secret via env vars (em IAM User dedicado) |
| Download de anexos | Proxy via PKD (sem pre-signed URLs) |
| Imagens inline do editor | Vão para S3 junto com os demais anexos |
| Thumbnails | Fora do escopo (feature separada para o futuro) |
| Migração | Síncrona, não-destrutiva, com verificação SHA256 |
| VPC Endpoint para S3 | Obrigatório em produção (gratuito) |

---

## 1.5 Cenário operacional

| Ambiente | Hospedagem | Storage hoje | Restrições |
|---|---|---|---|
| **Produção** | Instância EC2 (Linux) | EBS gp3 (~$0,08/GB/mês, dimensionado) | Crescimento de anexos consome EBS provisionado; redimensionar é manual e cresce custo linearmente |
| **Homologação/Dev** | UNRAID self-hosted | Filesystem local (custo do array UNRAID) | Sem credenciais AWS estáveis; precisa funcionar offline; idealmente sem dependência de cloud |

**Implicações desse cenário:**

1. **EC2 + S3 na mesma região não tem custo de transferência.** Esse fato neutraliza o principal argumento contra S3 em outros contextos.
2. **Produção precisa rodar 24/7 sem intervenção.** Auth via IAM Role evita rotação manual de credenciais.
3. **O mesmo binário precisa rodar em prod (EC2) e dev (UNRAID).** Configuração ambient-driven.
4. **Volume atual é trivial (8.7MB / 94 arquivos).** A motivação não é custo presente, é o crescimento futuro e a higiene operacional.

---

## 2. Alternativas avaliadas

| Alternativa | Custo @1TB/mês | Escalabilidade | Egress p/ EC2 | Auth EC2 | Conclusão |
|---|---:|---|---|---|---|
| **EBS gp3 (status quo)** | $80 | Manual (resize) | n/a | n/a | Caro no longo prazo, não escala automaticamente. |
| **AWS S3 Intelligent-Tiering** | $13–$23 | Automática, ilimitada | **Grátis** (mesma região) | **IAM Role** | ✅ **Selecionado.** |
| **AWS S3 Standard** | $23 | Automática | Grátis | IAM Role | Funciona, mas Intelligent-Tiering é mais econômico sem desvantagens. |
| **EFS Standard (NFS)** | $300 | Automática | Grátis | IAM Role | ❌ 13× mais caro que S3 e mais lento. |
| **Cloudflare R2** | $15 | Automática | **Pago** (egress AWS→R2) | Access Key | ❌ Em prod EC2, paga-se egress AWS para sair. |
| **MinIO em outra EC2** | Custo de outra EC2 + EBS | Manual | Grátis | Access Key | ❌ Reinventa S3 pagando para isso. |

Conclusão: **S3 Intelligent-Tiering é objetivamente melhor que todas as alternativas no cenário descrito.**

---

## 3. Estado atual do projeto

### 3.1 Arquitetura de anexos

| Aspecto | Implementação atual |
|---|---|
| Persistência | Filesystem local sob `PKD_ATTACHMENTS_PATH` (`internal/store/attachments.go`) |
| Layout | Sharded: `<base>/[inline/]ab/cd/<token-22ch>` (token = `security.NewToken(16)`) |
| Metadata | Tabela `attachments(id, document_id, original_name, stored_filename, mime_type, size_bytes, created_at)` |
| Upload | `multipart/form-data` (painel) ou `application/octet-stream` (CKEditor inline) → `io.Copy(file, body)` com limite |
| Download | `http.ServeContent` lê do disco, suporta **HTTP Range** (essencial para PDFs/vídeos) |
| Path traversal | `security.SafeAttachmentPath` valida `..`, null bytes, paths absolutos |
| Limpeza | `ListOrphanedStoredFiles` faz `filepath.Walk` e compara com a tabela |
| Acoplamento | Direto: `os.Create`, `os.Open`, `os.Remove`, `os.MkdirAll`, `filepath.Walk` |

**Ponto-chave:** Não existe abstração de storage. O `AttachmentStore` chama diretamente o `os` package. **Esta é a refatoração obrigatória antes de qualquer outra coisa.**

### 3.2 Configuração

- `internal/config/config.go` lê tudo de `os.Getenv`. Não há tabela de settings.
- Variáveis atuais relevantes: `PKD_ATTACHMENTS_PATH`, `PKD_MAX_ATTACHMENT_MB`, `PKD_MAX_IMAGE_MB`.
- Distroless static-debian12 → CA roots embutidos, AWS SDK em Go puro funciona sem CGO.

### 3.3 Tamanho do código a refatorar

`AttachmentStore` tem 240 linhas. Pontos a tocar:
- `CreateFile` (escreve no disco)
- `Delete` (remove do disco)
- `DeleteByDocument` (remove em lote)
- `ListOrphanedStoredFiles` (varre disco)
- `FullPath` + `handleGetAttachment` (abre arquivo para servir)
- `GetByID`, `ListByDocument`, `ListAllWithDocument` permanecem inalterados (só metadata)

---

## 4. Desenho da solução

### 4.1 Escolha do backend no painel de administração

A escolha é persistida na tabela nova `settings(key, value)` no SQLite, com chave `attachments.backend = "local" | "s3"`. **Credenciais nunca entram nessa tabela.**

O admin UI mostra:
- Backend ativo: `local` ou `s3`
- Status de configuração S3:
  - Em prod (IAM Role): "✅ Autenticado via Instance Profile `<role-name>`"
  - Em dev (env vars): "✅ Configurado via env vars" ou "❌ S3 não configurado"
- Bucket / Region / Prefix (não-segredos, ok mostrar)
- Última verificação (quando, status, latência)

### 4.2 Variáveis de configuração

**Em produção (EC2):**

```
PKD_S3_BUCKET=pkd-prod-attachments
PKD_S3_PREFIX=                    # opcional, default vazio
PKD_S3_REGION=us-east-1           # ou a região onde a EC2 roda
# Sem Access Key / Secret — SDK descobre via IMDS automaticamente
```

**Em dev/homologação (UNRAID), opcional:**

```
PKD_S3_ACCESS_KEY_ID=AKIA...
PKD_S3_SECRET_ACCESS_KEY=...
PKD_S3_BUCKET=pkd-dev-attachments
PKD_S3_PREFIX=
PKD_S3_REGION=us-east-1
```

(O caso comum em dev é manter `attachments.backend = "local"` — UNRAID não precisa de S3 nenhum. Mas a opção existe para testar contra um bucket de dev real.)

### 4.3 Modelo de autenticação

#### Em produção (EC2 + IAM Role)

1. Criar IAM Role `pkd-prod-attachments-role` com policy mínima:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [{
       "Effect": "Allow",
       "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
       "Resource": "arn:aws:s3:::pkd-prod-attachments/*"
     }, {
       "Effect": "Allow",
       "Action": ["s3:ListBucket"],
       "Resource": "arn:aws:s3:::pkd-prod-attachments"
     }]
   }
   ```
2. Anexar role à instância EC2.
3. Habilitar **IMDSv2-only** (`HttpTokens=required`) na EC2.
4. **Nenhuma credencial em env var, log, SQLite ou backup.**

#### Em desenvolvimento (UNRAID + Access Key)

- Criar IAM User dedicado (`pkd-dev`), Access Key + Secret.
- Bucket separado (`pkd-dev-attachments`).
- Policy análoga, restrita ao bucket de dev.
- Env vars no UNRAID Docker.
- Rotacionar a key periodicamente (manual; documentar em `docs/operations.md`).

#### Como o código decide automaticamente

```go
cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(s3Region))
// AWS SDK v2 tenta, em ordem:
//   1. Env vars (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
//   2. Shared credentials file
//   3. EC2 Instance Metadata Service (IMDS)
// Em prod EC2 sem env vars → cai automaticamente em IMDS.
// Em UNRAID com env vars → usa as env vars.
```

PKD seta `AWS_ACCESS_KEY_ID` e `AWS_SECRET_ACCESS_KEY` no `os.Setenv` a partir de `PKD_S3_*` antes de chamar `LoadDefaultConfig`.

#### Princípios universais

| Controle | Decisão |
|---|---|
| Persistência das credenciais | Apenas em env vars (dev) ou IMDS (prod). **Nunca no SQLite.** |
| Exposição na UI | Admin UI **nunca** lê nem exibe segredos. Mostra só metadados. |
| Logs | Sanitizar erros do AWS SDK antes de logar. |
| Backups SQLite | Sem credenciais → backups continuam livres de segredos. |
| Server-Side Encryption | **SSE-S3** (`AES256`) por padrão em todos os PUTs. Custo zero, transparente. |
| Bucket privado | `Block all public access` ativado. |
| Versioning | Habilitado, com lifecycle "remover versões não-correntes >90 dias". |

### 4.4 VPC Endpoint para S3 (produção)

Criar um **Gateway Endpoint para S3** na VPC onde a EC2 reside:

| Benefício | Impacto |
|---|---|
| Gratuito | Gateway endpoints para S3 não cobram |
| Tráfego interno AWS | Não sai para a internet, latência ~1-5ms |
| Sem NAT Gateway | Se a VPC usa NAT Gateway, evita $0,045/GB processado + $0,045/hora |
| Restrição via policy | Pode-se limitar quais buckets a VPC alcança |

**Sempre habilitar em prod.**

### 4.5 S3 Intelligent-Tiering

Configurar Intelligent-Tiering como storage class default do bucket. Comportamento:

- Objetos novos entram em **Frequent Access** ($0,023/GB/mês).
- Após 30 dias sem acesso → AWS move para **Infrequent Access** ($0,0125/GB/mês).
- Após 90 dias sem acesso → **Archive Instant Access** ($0,004/GB/mês).
- Acesso **sempre imediato** (latência idêntica ao Standard) em qualquer tier.
- Custo de monitoramento: $0,0025 por 1.000 objetos/mês (irrelevante).

Sem necessidade de lifecycle rules para tiering — a AWS gerencia. Lifecycle rule única configurada: **remover versões antigas (não-correntes) após 90 dias**, para limitar crescimento do Versioning.

### 4.6 Migração inicial (8.7MB / 94 arquivos)

**Drasticamente simplificada pelo volume mínimo.** A operação inteira (ler do EBS, calcular SHA256, escrever no S3, verificar) leva **menos de 30 segundos** para todo o dataset atual.

**Implementação:**

1. Adicionar coluna `attachments.storage_location TEXT NOT NULL DEFAULT 'local'`.
2. Adicionar coluna `attachments.content_sha256 TEXT`.
3. Endpoint `POST /api/admin/storage/migrate` é **síncrono**:
   - Lista todos `attachments` onde `storage_location != target`.
   - Para cada um: lê origem → calcula SHA256 → escreve destino → re-lê para validar SHA256 → atualiza linha em transação.
   - Não deleta da origem.
   - Retorna resumo: `{copied, errors[], duration_ms}`.
4. Frontend mostra spinner; recebe resposta em segundos.
5. Botão separado **"Limpar arquivos da origem"** roda passada simples removendo o que está em `local` mas marcado como migrado.

Sem job persistido, sem retomada, sem goroutine background. **Não precisa.**

(Nota: se no futuro o volume crescer para escala que exija >10 minutos de migração, a coluna `storage_location` por linha já garante idempotência — basta repetir a chamada e ela retoma de onde parou. A complexidade resumível pode ser adicionada quando — e se — for necessária.)

### 4.7 Verificação de conexão

`POST /api/admin/storage/test`:

**Local:** `os.Stat` + escrever/ler/remover arquivo de teste em `<base>/.pkd-healthcheck`.

**S3:** sequência mínima:
1. `HeadBucket` → bucket existe e auth funciona.
2. `PutObject` em `<prefix>/.pkd-healthcheck-<timestamp>` (1 byte).
3. `GetObject` → comparar bytes.
4. `DeleteObject`.
5. Reportar latência total e por step.

Custo por verificação: <$0,000020. Rate limit: 1/min.

### 4.8 Imagens inline do editor

Vão para o S3 junto com os demais anexos. Sem caminho separado no código. O subdir `inline/` permanece apenas como prefixo dentro do bucket S3 (`<prefix>/inline/ab/cd/<token>`), preservando organização visual ao listar via console AWS.

### 4.9 Thumbnails

**Fora do escopo desta feature.** Será tratado em projeto separado quando/se demandado. PKD não gera thumbnails hoje.

---

## 5. Custos estimados (cenário produção, us-east-1)

### Hoje (EBS):
```
EBS gp3 provisionado ~30GB → $2,40/mês (capacidade total da instância)
Snapshots EBS diários: ~$0,30/mês
```
*Anexos representam fatia minúscula deste custo.*

### Após implementação (S3 Intelligent-Tiering):
```
8.7MB armazenados → $0,0002/mês (literalmente um décimo de centavo)
Requests: <$0,01/mês
Egress EC2→S3: $0
EBS pode ser reduzido (mas só vale se o EBS hoje estiver dimensionado por causa dos anexos)
Total adicional pelo S3: ~$0,01/mês
```

### Crescimento projetado (1 ano @ 100MB):
```
100MB ≈ $0,002/mês. Continua irrelevante.
```

### Crescimento de longo prazo (5 anos @ 50GB):
```
S3 Intelligent-Tiering misto: ~$1/mês.
Equivalente em EBS: $4/mês + necessidade de redimensionar volume.
```

**Conclusão honesta:** o argumento financeiro **não é o motivador principal no curto prazo** — a economia atual é desprezível. O argumento real é **escalabilidade automática + higiene operacional + segurança superior via IAM Role**. Em 5-10 anos, com crescimento contínuo, o argumento financeiro também se materializa.

---

## 6. Mudanças concretas necessárias

### 6.1 Refatoração da camada de storage

Criar `internal/storage/`:

```go
type Storage interface {
    Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error)
    Stat(ctx context.Context, key string) (StorageObject, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
}
```

Implementações:
- `internal/storage/local.go` — wrap do código atual.
- `internal/storage/s3.go` — usando `github.com/aws/aws-sdk-go-v2/service/s3`.

`AttachmentStore` passa a depender de `Storage` (DI). `handleGetAttachment` ganha suporte a Range manual (parsear `Range` header → `GetRange` → escrever 206 Partial Content). Para v1, pode-se aceitar streaming sem Range para S3 — visualizadores caem em fallback gracioso. Range adapter pode ser v2.

### 6.2 Schema do banco

```sql
ALTER TABLE attachments ADD COLUMN storage_location TEXT NOT NULL DEFAULT 'local';
ALTER TABLE attachments ADD COLUMN content_sha256 TEXT;

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

(Sem `migration_jobs` — não há migração assíncrona para rastrear.)

### 6.3 Configuração

`internal/config/config.go` ganha:

```go
type S3Config struct {
    Bucket          string
    Prefix          string
    Region          string
    AccessKeyID     string  // opcional — vazio em prod com IAM Role
    SecretAccessKey string  // opcional — idem
}
S3 *S3Config  // nil quando bucket/region não definidos
```

Validação: se `PKD_S3_BUCKET` e `PKD_S3_REGION` definidos → S3 está configurado. Se também houver Access Key → usa essas credenciais. Senão, deixa o SDK descobrir via IMDS.

### 6.4 Handlers novos

```
GET    /api/admin/storage/config          → {backend, s3_configured, s3_bucket, s3_region, auth_method}
PUT    /api/admin/storage/config          → {backend: "local"|"s3"}
POST   /api/admin/storage/test            → {local: {ok, latency_ms}, s3: {ok, latency_ms, error}}
POST   /api/admin/storage/migrate         → síncrono, retorna {copied, errors[], duration_ms}
POST   /api/admin/storage/cleanup-source  → remove arquivos da origem após migração validada
```

### 6.5 Frontend

Nova aba/painel em `/admin` (Svelte 5 + a stack já existente). Sem dependências novas. Componentes:
- Indicador de backend ativo + toggle.
- Card de status S3 (configurado/não, bucket, region, auth method).
- Botões "Verificar conexão", "Migrar agora", "Limpar origem".
- Output de cada operação (último resultado).

### 6.6 Documentação

- **Novo:** `docs/s3-storage.md` — guia completo: criar bucket, IAM Role, IAM User para dev, VPC Endpoint, lifecycle, migração, troubleshooting.
- `docs/operations.md` — adicionar seção sobre rotação de Access Key de dev e monitoramento de custos S3.
- `docs/security.md` — nova seção "Anexos em S3" descrevendo modelo de auth (IAM Role em prod, env vars em dev), SSE, bucket privado.
- `docs/unraid-install.md` — seção opcional "Apontar UNRAID para bucket S3 de dev".

### 6.7 Dependências novas

- `github.com/aws/aws-sdk-go-v2`
- `github.com/aws/aws-sdk-go-v2/config`
- `github.com/aws/aws-sdk-go-v2/credentials`
- `github.com/aws/aws-sdk-go-v2/service/s3`

Tamanho do binário: +8-12 MB stripped. Aceitável.

### 6.8 Operações em produção (checklist deploy)

1. Criar bucket `pkd-prod-attachments` na região da EC2.
2. Bloquear acesso público (`Block all public access` ativado).
3. Habilitar **SSE-S3** (criptografia em repouso) como default.
4. Habilitar **Versioning**.
5. Configurar **Intelligent-Tiering** como default storage class (Bucket → Properties → Default storage class).
6. Lifecycle rule: deletar versões não-correntes após 90 dias.
7. Criar IAM Role `pkd-prod-attachments-role` com policy mínima.
8. Anexar role à instância EC2 (Instance Profile).
9. Forçar **IMDSv2-only** (`HttpTokens=required`).
10. Criar **Gateway Endpoint para S3** na VPC.
11. Setar env vars `PKD_S3_BUCKET`, `PKD_S3_REGION` no container PKD.
12. Re-deploy PKD.
13. Verificar via `/api/admin/storage/test`.
14. Trocar backend para `s3` via admin UI.
15. Iniciar migração via `/api/admin/storage/migrate` (síncrona, segundos).
16. Após sucesso: `/api/admin/storage/cleanup-source`.
17. Configurar **AWS Budget alert** (alerta por email se gasto >$5/mês).

---

## 7. Análise de riscos

| Risco | Severidade | Probabilidade | Mitigação |
|---|:---:|:---:|---|
| Vazamento de credenciais (em dev/UNRAID) | Alta | Baixa | Sanitizar logs; bucket de dev isolado; rotação documentada |
| Vazamento via IMDS sequestrado (prod) | Alta | Muito baixa | IMDSv2-only; policy IAM mínima; CloudTrail |
| Custo S3 inesperado | Baixa | Muito baixa | AWS Budget alert; volume atual é insignificante |
| Migração com erro silencioso | Média | Baixa | Verificação SHA256 obrigatória após cada cópia |
| Range request não suportado em PDFs grandes | Média | Média | v1: streaming; v2: Range adapter |
| Latência percebida em downloads | Baixa | Alta | EC2↔S3 mesma região = ~5-10ms; aceitável |
| Delete acidental no bucket | Média | Baixa | S3 Versioning protege por 90 dias |
| AWS SDK quebra build sem CGO | Baixa | Muito baixa | SDK v2 é pure Go; CI valida |
| EC2 sem permissão na role | Média | Média | Endpoint `/api/admin/storage/test` detecta antes do tráfego real |

---

## 8. Decisões confirmadas (referência rápida)

| Tema | Decisão |
|---|---|
| 1. Volume atual | 94 arquivos / 8.7MB → migração síncrona é trivial |
| 2. Storage class | S3 Intelligent-Tiering |
| 3. Versioning | Habilitado, lifecycle removendo versões >90 dias |
| 4. Buckets | Separados: `pkd-prod-attachments`, `pkd-dev-attachments` |
| 5. Download | Proxy via PKD (sem pre-signed URLs) |
| 6. Inline images | Vão para S3 junto |
| 7. Thumbnails | Fora do escopo desta feature |
| 8. Migração | Síncrona, não-destrutiva, com verificação SHA256 |
| 9. Auth prod | IAM Role do Instance Profile |
| 10. Auth dev | IAM User com Access Key em env var |
| 11. VPC Endpoint | Gateway Endpoint para S3 (gratuito, obrigatório em prod) |
| 12. SSE | SSE-S3 (AES256) em todos os PUTs |
| 13. Range requests | v1: sem suporte (streaming simples); v2: implementar se demanda |

---

## 9. Resumo de complexidade e esforço

| Componente | Complexidade | Esforço |
|---|---|---|
| Refatorar `AttachmentStore` para `Storage` interface | Média | 1 dia |
| Implementar `LocalStorage` (wrap do existente) | Baixa | 0,25 dia |
| Implementar `S3Storage` (com auth automático IMDS/env) | Média | 1 dia |
| Configuração + admin endpoints (config, test) | Baixa | 0,5 dia |
| Migração síncrona com SHA256 + cleanup-source | Baixa | 0,5 dia |
| Frontend admin (Svelte) | Baixa | 0,5 dia |
| Documentação (s3-storage.md, operations, security) | Baixa | 0,75 dia |
| Testes (unit + integração com MinIO local em CI) | Média | 0,75 dia |
| **Total v1** | | **~5 dias** (margem confortável) |

**v2 opcional (apenas se necessário no futuro):**
- Range requests para S3: +0,5 dia
- Multipart upload (>5MB): +0,5 dia
- Migração resumível: +1,5 dia (só se volume crescer muito)

---

## 10. Próximos passos

1. **Validar o relatório** — última oportunidade de mudar decisões antes da spec formal.
2. **Produzir a especificação técnica** em `specs/006-s3-storage/spec.md` seguindo o padrão das specs anteriores (003, 004, 005).
3. **Implementar v1** seguindo o plano em §6 e o checklist de deploy em §6.8.
4. **Provisionar AWS** em paralelo seguindo os Apêndices A–G (essas etapas podem rodar antes do código estar pronto).
5. **Deploy gradual:** dev/UNRAID continua com backend `local` por padrão. Produção EC2 ativa o backend `s3` via admin UI após verificação OK.

---

# Apêndices — Passo-a-passo no Console AWS

> Estes apêndices assumem **zero familiaridade prévia** com IAM, VPC e Endpoints. Cada apêndice descreve a operação inteira, do login no console até a verificação final. Onde dou um nome (`pkd-prod-attachments`, `pkd-prod-attachments-role` etc.), você pode usar qualquer outro — mas mantenha consistência em todos os passos.
>
> **Vocabulário básico antes de começar:**
> - **Console AWS:** o site `https://console.aws.amazon.com/`. Todas as operações abaixo são feitas lá.
> - **Região (Region):** datacenter geográfico (ex: `us-east-1` = Norte da Virgínia, `sa-east-1` = São Paulo). **A região da EC2 e a região do bucket S3 devem ser a mesma** para o tráfego ser gratuito. No canto superior direito do console, sempre confirme qual região está selecionada.
> - **IAM:** "Identity and Access Management" — o serviço da AWS que controla quem pode fazer o quê.
> - **VPC:** "Virtual Private Cloud" — a "rede privada" onde sua EC2 vive.
> - Quando eu disser "navegue até o serviço X", o caminho é: barra de busca no topo → digite o nome → clique no resultado.

---

## Apêndice A — Criar e configurar o bucket S3 de produção

**O que vamos fazer:** criar o bucket `pkd-prod-attachments`, bloquear acesso público, ativar criptografia em repouso, ativar Versioning, configurar Intelligent-Tiering como classe padrão, e adicionar uma regra de ciclo de vida para limpar versões antigas.

**Tempo estimado:** 10 minutos.

### A.1 Criar o bucket

1. Console AWS → barra de busca → **S3** → clique no resultado.
2. Confirme no topo direito que a região é a **mesma da sua EC2** (ex: `us-east-1`). Se não for, mude.
3. Clique em **Create bucket**.
4. **Bucket name:** `pkd-prod-attachments`
   *(precisa ser globalmente único na AWS inteira; se estiver tomado, adicione um sufixo: `pkd-prod-attachments-edalcin`)*
5. **AWS Region:** confirme.
6. **Object Ownership:** mantenha **ACLs disabled (recommended)**.
7. **Block Public Access settings for this bucket:** mantenha **todas as 4 caixas marcadas** (esse é o estado padrão). Confirme a checkbox de aviso embaixo.
8. **Bucket Versioning:** marque **Enable**.
9. **Default encryption:**
   - **Encryption type:** `Server-side encryption with Amazon S3 managed keys (SSE-S3)`.
   - **Bucket Key:** `Enable` *(reduz custo de criptografia, transparente)*.
10. **Advanced settings:** deixe como está (sem Object Lock).
11. Clique em **Create bucket** no fim da página.

✅ Bucket criado com criptografia + Versioning. Próximo passo: classe de armazenamento.

### A.2 Configurar Intelligent-Tiering como classe padrão

A AWS não tem um botão direto "tornar Intelligent-Tiering o padrão do bucket" — precisa ser configurado via lifecycle rule (que aplica Intelligent-Tiering a partir do dia 0).

1. Na página do bucket → aba **Management** → seção **Lifecycle rules** → **Create lifecycle rule**.
2. **Lifecycle rule name:** `default-intelligent-tiering`.
3. **Choose a rule scope:** `Apply to all objects in the bucket`. Marque a confirmação.
4. **Lifecycle rule actions:** marque **Transition current versions of objects between storage classes**.
5. Em **Transition current versions of objects between storage classes**:
   - **Storage class transitions:** `Intelligent-Tiering`.
   - **Days after object creation:** `0`.
6. Clique **Create rule**.

✅ Todo objeto novo vai direto para Intelligent-Tiering.

### A.3 Lifecycle para limpar versões antigas (Versioning)

Mesma tela de Lifecycle rules → **Create lifecycle rule** novamente.

1. **Lifecycle rule name:** `cleanup-old-versions`.
2. **Choose a rule scope:** `Apply to all objects in the bucket`. Confirme.
3. **Lifecycle rule actions:** marque **Permanently delete noncurrent versions of objects**.
4. **Days after objects become noncurrent:** `90`.
5. **Number of newer versions to retain:** deixe vazio (ou `1`).
6. Clique **Create rule**.

✅ Versões antigas (de objetos modificados/deletados) somem após 90 dias. Você está protegido por 90 dias contra qualquer delete acidental.

### A.4 Repetir para o bucket de dev

Repetir A.1, A.2, A.3 trocando o nome para `pkd-dev-attachments`. (Ou, se o ambiente de dev não vai usar S3 inicialmente, pular este passo e fazer quando precisar.)

---

## Apêndice B — Criar a IAM Role para a EC2 de produção

**O que vamos fazer:** criar uma "credencial automática" que a EC2 carrega sem precisar de Access Key. Isso envolve três objetos AWS:

1. **IAM Policy** — documento JSON que diz "pode fazer X em Y".
2. **IAM Role** — uma identidade que pode ser assumida por um serviço (no caso, EC2). A Policy é anexada à Role.
3. **Instance Profile** — wrapper que conecta a Role à instância EC2. (No console moderno, criado automaticamente quando você cria a Role com tipo "EC2".)

**Tempo estimado:** 8 minutos.

### B.1 Criar a IAM Policy

1. Console AWS → busca → **IAM** → clique.
2. Menu lateral esquerdo → **Policies** → **Create policy**.
3. Aba **JSON** (no topo, ao lado de "Visual"). Cole exatamente isto, **trocando `pkd-prod-attachments` pelo nome do seu bucket** se for diferente:

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Sid": "ReadWriteObjects",
         "Effect": "Allow",
         "Action": [
           "s3:GetObject",
           "s3:PutObject",
           "s3:DeleteObject"
         ],
         "Resource": "arn:aws:s3:::pkd-prod-attachments/*"
       },
       {
         "Sid": "ListBucket",
         "Effect": "Allow",
         "Action": ["s3:ListBucket"],
         "Resource": "arn:aws:s3:::pkd-prod-attachments"
       }
     ]
   }
   ```

   *(Note os dois ARNs: o primeiro com `/*` no fim refere-se aos objetos dentro do bucket; o segundo, sem barra, refere-se ao bucket em si. Os dois são necessários.)*

4. **Next**.
5. **Policy name:** `pkd-prod-attachments-policy`.
6. **Description:** "Acesso de leitura/escrita aos objetos do bucket pkd-prod-attachments".
7. **Create policy**.

✅ Policy criada.

### B.2 Criar a IAM Role

1. Menu lateral IAM → **Roles** → **Create role**.
2. **Trusted entity type:** `AWS service`.
3. **Service or use case:** `EC2`.
4. **Use case:** `EC2` (a opção padrão).
5. **Next**.
6. Na lista de policies, busque por `pkd-prod-attachments-policy` → marque a checkbox.
7. **Next**.
8. **Role name:** `pkd-prod-attachments-role`.
9. **Description:** "Permite à EC2 do PKD ler/escrever no bucket de anexos".
10. Revise — em **Trusted entities**, deve aparecer `ec2.amazonaws.com`. Em **Permissions**, deve aparecer a policy do passo B.1.
11. **Create role**.

✅ Role criada. Um Instance Profile com o mesmo nome foi criado automaticamente em segundo plano.

### B.3 Anexar a Role à instância EC2

1. Console AWS → busca → **EC2** → clique.
2. Confirme a região.
3. Menu lateral → **Instances** → marque a checkbox da sua instância PKD.
4. Botão **Actions** (no topo) → **Security** → **Modify IAM role**.
5. **IAM role:** dropdown → selecione `pkd-prod-attachments-role`.
6. **Update IAM role**.

✅ A instância agora tem credenciais automáticas para o bucket. **Não precisa reiniciar a EC2** — a credencial está disponível imediatamente via IMDS.

### B.4 Verificar que funcionou (opcional, mas recomendado)

SSH na EC2 e rode:

```bash
# Pegar token IMDSv2
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 60")

# Ver qual role está anexada
curl -s -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/iam/security-credentials/

# Deve retornar: pkd-prod-attachments-role
```

Se o AWS CLI estiver instalado:
```bash
aws s3 ls s3://pkd-prod-attachments/
# Deve listar (vazio é ok), não deve dar erro de permissão.
```

---

## Apêndice C — Habilitar IMDSv2-only na EC2

**O que é:** o **Instance Metadata Service** (IMDS) é o "telefone interno" da EC2 — quando o código pede credenciais via SDK, ele liga para `http://169.254.169.254/...` e recebe a credencial temporária. Existem duas versões do protocolo:

- **IMDSv1:** qualquer requisição HTTP simples ao endereço retorna a credencial. Vulnerável a SSRF (se um atacante conseguir fazer seu app fazer um GET para esse endereço, ele rouba a credencial).
- **IMDSv2:** exige um token PUT antes do GET. Imune a SSRF acidental.

**O que vamos fazer:** forçar a EC2 a aceitar somente IMDSv2.

**Tempo estimado:** 2 minutos.

### C.1 Pelo Console

1. Console AWS → **EC2** → **Instances** → marque a instância PKD.
2. **Actions** → **Instance settings** → **Modify instance metadata options**.
3. **Instance metadata service:** `Enable`.
4. **IMDSv2:** `Required` *(esta é a opção crítica)*.
5. **Metadata response hop limit:** `1` (default; suficiente — só o processo na EC2 acessa).
6. **Metadata tags:** mantenha desabilitado.
7. **Save**.

✅ A partir de agora, qualquer tentativa de IMDSv1 retorna 401. O AWS SDK em Go já fala IMDSv2 por padrão, então o PKD continua funcionando normalmente.

### C.2 Verificar

SSH na EC2:

```bash
# IMDSv1 deve falhar:
curl -s -o /dev/null -w "%{http_code}\n" http://169.254.169.254/latest/meta-data/
# Esperado: 401

# IMDSv2 deve funcionar:
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 60")
curl -s -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/
# Esperado: lista de campos
```

---

## Apêndice D — Criar VPC Gateway Endpoint para S3

**O que é (didático):** quando a EC2 faz uma chamada para `s3.us-east-1.amazonaws.com`, normalmente o pacote sai pela rede pública da AWS. Em VPCs com **NAT Gateway**, esse pacote passa pelo NAT (que cobra $0,045/GB processado).

Um **Gateway Endpoint para S3** é uma "rota mágica" adicionada à tabela de rotas da VPC: ela diz "tráfego para S3 não vai pela internet, vai por um caminho privado interno da AWS". É **gratuito**, mais rápido (~1-5ms a menos de latência) e mais seguro.

**O que vamos fazer:** criar o endpoint e associá-lo à tabela de rotas da subnet onde a EC2 vive.

**Tempo estimado:** 5 minutos.

### D.1 Identificar a VPC e o Route Table da sua EC2

1. Console AWS → **EC2** → **Instances** → clique na sua instância (não só marque, abra os detalhes).
2. Aba **Networking** → anote dois valores:
   - **VPC ID:** `vpc-XXXXXXXX`
   - **Subnet ID:** `subnet-XXXXXXXX`
3. Vá para **VPC** (busca no topo).
4. Menu lateral → **Subnets** → procure pelo Subnet ID anotado → clique.
5. Aba **Route table** → anote o **Route table ID** (`rtb-XXXXXXXX`).

### D.2 Criar o Endpoint

1. No console **VPC** → menu lateral → **Endpoints** → **Create endpoint**.
2. **Name tag:** `pkd-s3-gateway-endpoint`.
3. **Service category:** `AWS services`.
4. **Services:** na busca, digite `s3`. **Filtre por "Type: Gateway"** (importante — há também "Interface", que é pago e não é o que queremos). Selecione `com.amazonaws.<region>.s3` com **Type: Gateway**.
5. **VPC:** selecione a VPC anotada (`vpc-XXXXXXXX`).
6. **Route tables:** marque a checkbox da route table anotada (`rtb-XXXXXXXX`).
7. **Policy:** mantenha **Full access** *(ou, mais restritivo, ver D.4 abaixo)*.
8. **Create endpoint**.

✅ Endpoint criado. Em segundos, a route table ganha automaticamente uma rota para o serviço S3 via o endpoint.

### D.3 Verificar

```bash
# Na EC2:
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 60")
REGION=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/placement/region)
nslookup s3.$REGION.amazonaws.com
# O IP retornado deve estar em 10.x.x.x ou similar (faixa privada),
# não em IPs públicos da AWS.
```

Ou via console: VPC → Route Tables → sua route table → aba **Routes** → deve aparecer uma linha com `Destination: pl-XXXXXX (com.amazonaws.<region>.s3)` e `Target: vpce-XXXXXX`.

### D.4 Restringir a policy do Endpoint (opcional, recomendado)

Por padrão, o endpoint permite acesso a **qualquer bucket S3 no mundo**. Para restringir apenas aos seus buckets:

1. VPC → Endpoints → seu endpoint → aba **Policy** → **Edit policy**.
2. Cole:

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [{
       "Effect": "Allow",
       "Principal": "*",
       "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"],
       "Resource": [
         "arn:aws:s3:::pkd-prod-attachments",
         "arn:aws:s3:::pkd-prod-attachments/*"
       ]
     }]
   }
   ```

3. **Save**.

Agora qualquer processo na EC2 só consegue tocar nos buckets do PKD via o endpoint. **Defesa em profundidade.**

---

## Apêndice E — Configurar AWS Budget alert

**Por que:** garante que se algum dia o custo do PKD na AWS sair do esperado (ex: configuração errada gerando milhões de requisições, ou alguém fazendo upload massivo), você recebe um email.

**Tempo estimado:** 3 minutos.

1. Console AWS → busca → **Billing and Cost Management** → menu lateral → **Budgets** → **Create budget**.
2. **Budget setup:** `Customize (advanced)`.
3. **Budget type:** `Cost budget — Recommended`.
4. **Next**.
5. **Budget name:** `pkd-monthly-budget`.
6. **Period:** `Monthly`.
7. **Budget effective date:** `Recurring budget`.
8. **Budget amount — Enter your budgeted amount:** `5` (USD/mês — ajuste ao gosto).
9. **Budget scope:** `All AWS services` (ou filtre por tag/serviço se quiser ser específico).
10. **Next**.
11. **Configure alerts:**
    - Threshold: `80` % of budgeted amount.
    - Trigger: `Actual`.
    - Email recipients: seu email.
12. **Next** → revise → **Create budget**.

✅ Você recebe email se o gasto AWS atingir 80% de $5/mês.

---

## Apêndice F — Criar IAM User para o ambiente de dev (UNRAID)

**Por que:** o UNRAID não roda em EC2, então não tem acesso a IAM Role. Precisa de uma Access Key estática para falar com S3.

**Tempo estimado:** 5 minutos.

### F.1 Criar a policy de dev

Repetir Apêndice B.1, mas trocando `pkd-prod-attachments` por `pkd-dev-attachments` em todo o JSON. Nome: `pkd-dev-attachments-policy`.

### F.2 Criar o IAM User

1. Console AWS → **IAM** → **Users** → **Create user**.
2. **User name:** `pkd-dev`.
3. **Provide user access to the AWS Management Console:** **deixe desmarcado** (esse user só vai usar API).
4. **Next**.
5. **Permissions options:** `Attach policies directly`.
6. Busque por `pkd-dev-attachments-policy` → marque.
7. **Next** → revise → **Create user**.

### F.3 Gerar Access Key

1. Na lista de Users → clique em `pkd-dev` → aba **Security credentials** → seção **Access keys** → **Create access key**.
2. **Use case:** `Application running outside AWS`.
3. Confirme o aviso → **Next**.
4. **Description tag value:** `pkd-dev on UNRAID`.
5. **Create access key**.
6. **CRÍTICO:** copie agora os dois valores — **Access key ID** e **Secret access key**. O Secret só é mostrado nesta tela; depois disso é impossível recuperá-lo (só gerar uma nova).
7. Cole no UNRAID Docker como variáveis:
   - `PKD_S3_ACCESS_KEY_ID=AKIA...`
   - `PKD_S3_SECRET_ACCESS_KEY=...`
   - `PKD_S3_BUCKET=pkd-dev-attachments`
   - `PKD_S3_REGION=us-east-1` (ou a região do bucket)

✅ UNRAID agora pode falar com o bucket de dev.

### F.4 Boa prática: rotacionar a key periodicamente

A cada 90-180 dias:
1. Crie uma segunda Access Key no mesmo user (até 2 simultâneas).
2. Atualize as env vars no UNRAID.
3. Verifique que o PKD continua funcionando.
4. Volte na key antiga → **Deactivate** → confirme que tudo continua funcionando → **Delete**.

---

## Apêndice G — Verificações finais (end-to-end)

Depois de fazer A–F, valide tudo na ordem:

| # | O quê | Como verificar | Resultado esperado |
|---|---|---|---|
| 1 | Bucket existe | Console S3 → lista de buckets | `pkd-prod-attachments` aparece |
| 2 | Bucket privado | Bucket → Permissions → Block public access | Tudo `On` |
| 3 | SSE ativada | Bucket → Properties → Default encryption | `SSE-S3` ativo |
| 4 | Versioning ativo | Bucket → Properties → Bucket Versioning | `Enabled` |
| 5 | Lifecycle ativo | Bucket → Management → Lifecycle rules | 2 regras: `default-intelligent-tiering` e `cleanup-old-versions` |
| 6 | Role anexada à EC2 | EC2 → instância → Security tab → IAM Role | `pkd-prod-attachments-role` |
| 7 | IMDSv2 obrigatório | EC2 → instância → Details → IMDSv2 | `Required` |
| 8 | Endpoint S3 ativo | VPC → Endpoints | `pkd-s3-gateway-endpoint` com Status `Available` |
| 9 | Route table tem rota S3 | VPC → Route Tables → sua route table → Routes | linha com `pl-...s3` apontando para `vpce-...` |
| 10 | EC2 consegue listar bucket | SSH na EC2: `aws s3 ls s3://pkd-prod-attachments/` | Lista (vazia ok), sem erro |
| 11 | IMDSv1 bloqueado | SSH: `curl -o /dev/null -w "%{http_code}" http://169.254.169.254/latest/meta-data/` | `401` |
| 12 | Budget alert criado | Billing → Budgets | `pkd-monthly-budget` com email configurado |

Se todos os 12 itens passarem, a infraestrutura AWS está pronta — basta o código do PKD ser deployed com `PKD_S3_BUCKET` e `PKD_S3_REGION` setados.

---

## Apêndice H — O que pedir ajuda direta vs. fazer sozinho

Você pode fazer **todos os apêndices A–G sozinho pelo Console AWS** — não há etapa que exija conhecimento de programação ou infraestrutura avançada.

**Onde provavelmente vai precisar de ajuda:**

| Situação | Sugestão |
|---|---|
| Não sabe em qual região sua EC2 está | Console → EC2 → canto superior direito → o nome da região está lá. Use a mesma para o bucket. |
| EC2 já tem outra IAM Role anexada | Não dá pra ter duas. Se a role atual é necessária para outras coisas, posso mostrar como **adicionar** a permissão de S3 à policy existente em vez de criar role nova. |
| VPC não tem NAT Gateway, todas as EC2 têm IP público | O Gateway Endpoint ainda vale a pena (latência menor, sem custo) mas o ganho é menor. Pode pular sem prejuízo funcional. |
| Erro "Access Denied" ao fazer `aws s3 ls` da EC2 | 9 em 10 vezes é a policy IAM com nome de bucket errado. Confirme que o nome no JSON da policy bate exatamente com o nome do bucket. |
| Não sabe como dar SSH na EC2 para verificações | Pode-se fazer todas as verificações sem SSH, via Console (S3 → Buckets, EC2 → instância details, VPC → Endpoints/Route Tables). As verificações via SSH são opcionais — só dão confirmação extra. |

**Quando me chamar de volta:**
- Antes de iniciar A–G, se quiser que eu confirme algum detalhe do seu setup atual (região, VPC, instância existente).
- Depois de A–G, com qualquer screenshot de erro — eu interpreto e indico a correção.
- Quando estiver pronto para o passo "implementar o código" — aí eu ajudo direto, escrevendo a refatoração e a integração com o AWS SDK.
