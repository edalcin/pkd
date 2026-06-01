# PKD Security Reference

Este documento descreve todos os controles de segurança do PKD, por que existem e o que não protegem.

---

## Modelo de ameaça

PKD é uma ferramenta **de usuário único, auto-hospedada**. O modelo de ameaça é:

- **Ameaça principal**: uma pessoa não autorizada tentando ler ou modificar suas notas pela rede.
- **Não é ameaça**: acesso físico ao servidor, sistema operacional comprometido, ou imagem Docker maliciosa.
- **Ameaça parcial**: outro dispositivo na mesma rede que observe o tráfego (mitigado por HTTPS via proxy reverso).

---

## Autenticação e gestão de sessão

| Controle | Mecanismo |
|---|---|
| Senha mestra | Fornecida via `PKD_PASSWORD` em runtime. Nunca armazenada no banco. Comparada com `crypto/subtle.ConstantTimeCompare` sobre SHA-256 de ambos os lados — previne timing attacks mesmo para strings de tamanhos diferentes. |
| Token de sessão | 32 bytes aleatórios criptograficamente seguros (base64url). Armazenado em mapa in-memory. Expira após `PKD_SESSION_IDLE_MINUTES` de inatividade (padrão: 60 min). Perdido no restart do container — usuário deve fazer login novamente. |
| Cookie de sessão | `HttpOnly; SameSite=Strict; Path=/`. Flag `Secure` intencionalmente ausente porque PKD frequentemente roda na LAN sem TLS. |
| Bloqueio por falhas | 5 tentativas incorretas do mesmo IP → bloqueio de 30 minutos. Contador reseta no login bem-sucedido. Header `Retry-After` indica o tempo de espera. |
| Detecção de IP | Por padrão `RemoteAddr`. Com `PKD_TRUST_PROXY_HEADERS=1` usa `X-Forwarded-For`. **Não ativar sem proxy reverso — permite spoofing de IP.** |

---

## Transporte

PKD não gerencia certificados TLS. Encerre TLS em um proxy reverso (Caddy, Traefik, UNRAID SWAG). Sem TLS, cookies de sessão trafegam em texto claro na LAN.

---

## CSRF

Padrão double-submit cookie:
- Em todo `GET`, se o cookie `pkd_csrf` estiver ausente, o servidor define um com 32 bytes aleatórios.
- Em toda requisição mutante (POST/PUT/DELETE/PATCH), o header `X-CSRF-Token` deve ser igual ao cookie `pkd_csrf`. Divergência → 403.
- O cookie CSRF **não é** HttpOnly para que o JavaScript possa lê-lo e incluí-lo no header.

---

## Content Security Policy

Dois CSPs distintos estão em uso:

| Escopo | Resumo da política |
|---|---|
| SPA autenticado (`/`, `/api/*`) | `script-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'` |
| View pública (`/public/{token}`) | `script-src 'none'; img-src 'self' data:; style-src 'self'; frame-ancestors 'none'` |

A view pública define `script-src 'none'` — nenhum JavaScript executa na página compartilhada. Mesmo que um payload malicioso bypass a sanitização HTML, não pode executar.

Headers em toda resposta: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Strict-Transport-Security: max-age=31536000; includeSubDomains`, `Permissions-Policy: interest-cohort=()`.

---

## Sanitização HTML

Todo HTML do editor passa pelo `bluemonday` antes do armazenamento e antes da renderização pública:

- **EditorPolicy**: permite formatação, imagens (com width inline), tabelas, links (http/https/mailto), blocos de código. Remove event handlers, `<script>`, `<style>`, URIs `javascript:` e `<foreignObject>` SVG.
- **PublicSharePolicy**: igual ao EditorPolicy, mas também remove estilos inline e atributos de dados.

Texto plano é derivado do HTML sanitizado para indexação FTS5.

**Captura de conteúdo**: o conteúdo recebido via `/api/capture` (POST JSON ou form-encoded) passa pela mesma sanitização antes de ser armazenado.

---

## Tokens de share link

- 32 bytes aleatórios via `crypto/rand`.
- Codificados como base64url (43 caracteres).
- Hash SHA-256 armazenado em `share_links.token_hash` — o plaintext **nunca é persistido**.
- `LookupByToken` compara hashes com `crypto/subtle.ConstantTimeCompare` para prevenir timing attacks.
- Revogação: define `revoked_at`. O endpoint público retorna 404 para tokens revogados ou inexistentes (nunca 401 ou 410 para não vazar existência).

---

## Extração de Open Graph (captura externa)

Quando `/api/capture` recebe uma URL, o servidor faz um HTTP GET externo:
- Timeout de 5 segundos.
- Corpo da resposta limitado a 1 MB.
- Operação best-effort — falha silenciosa; a captura prossegue sem metadados OG se a URL falhar.
- User-Agent identificável: `PKD/2.0 (+https://github.com/edalcin/pkd)`.

> **Nota**: este fetch não tem guarda SSRF (débito pré-existente). Veja seção abaixo para o fetch de imagens externas que tem proteção completa.

---

## Fetch de imagens externas (SSRF-safe)

Os endpoints `POST /api/documents/{id}/attachments/from-url` e `POST /api/admin/documents/{id}/import-external-images` permitem que o servidor busque URLs fornecidas pelo usuário autenticado. Isso é um vetor clássico de SSRF.

| Controle | Mecanismo |
|---|---|
| **Validação de scheme** | Apenas `http://` e `https://` são aceitos; qualquer outro scheme é rejeitado antes de abrir conexão. |
| **Bloqueio de IP pós-DNS** | `net.Dialer.Control` é chamado após resolução DNS, com o IP resolvido real. Rejeita: loopback (`IsLoopback`), privado RFC-1918 (`IsPrivate`), não-especificado `0.0.0.0`/`::` (`IsUnspecified`), link-local unicast/multicast (`IsLinkLocalUnicast`, `IsLinkLocalMulticast`), multicast (`IsMulticast`), `169.254.0.0/16` (metadata AWS/GCP — não coberto por `IsPrivate`) e `100.64.0.0/10` (CGNAT). Isso impede bypass via hostname que resolve para IP interno. |
| **Limite de redirects** | Máximo 5 redirects; cada destino é re-validado pelo mesmo `Control`. Scheme diferente de `http`/`https` em redirect é rejeitado. |
| **Validação de Content-Type** | Resposta com `Content-Type` que não comece com `image/` é rejeitada — impede uso do endpoint para exfiltrar conteúdo arbitrário (ex: HTML de serviços internos). |
| **Limite de tamanho** | `io.LimitReader` a `PKD_MAX_IMAGE_MB` — resposta maior retorna erro, sem consumo de memória excessivo. |
| **Timeout** | 10 segundos para conexão + leitura. |
| **Autenticação** | Endpoints requerem sessão autenticada (`AuthRequired` middleware). Usuário não autenticado não pode disparar fetches externos. |

---

## Geração de links públicos

A variável `PKD_BASE_URL` define o prefixo usado na geração de links públicos de compartilhamento. Se não definida, o servidor usa o host da requisição. Configure explicitamente em produção (ex: `https://pkd.dalc.in/`) para garantir que os links sejam sempre corretos, especialmente atrás de proxies reversos.

---

## Defesa contra path traversal em anexos

`security.SafeAttachmentPath(base, stored)`:
1. Rejeita string vazia.
2. Rejeita null bytes.
3. Rejeita caminhos absolutos Unix (`/…`) e Windows.
4. Rejeita componentes `..`.
5. Chama `filepath.Clean(filepath.Join(base, stored))` e verifica que o resultado está sob `base`.

---

## Banco de dados

- SQLite com `PRAGMA foreign_keys = ON` (integridade referencial).
- Modo WAL para leituras concorrentes durante backup.
- Todas as queries usam prepared statements — sem interpolação de strings em SQL.
- Arquivo de banco de dados fora do container em volume montado.

---

## Backup e restauração de anexos (S3)

| Controle | Mecanismo |
|---|---|
| **Autenticação** | Endpoints `/api/admin/storage/backup-start`, `/api/admin/storage/restore-start`, `/api/admin/storage/jobs/{id}` requerem sessão autenticada (middleware `AuthRequired`). Aplicação é single-user; sessão autenticada = admin. |
| **URL pré-assinada de download** | Gerada via `s3.NewPresignClient(client).PresignGetObject(..., s3.WithPresignExpires(15*time.Minute))`. TTL curto (15 min) limita a janela de exposição caso o link vaze em logs, proxies ou compartilhamento acidental. Após expirar, novo `POST /api/admin/storage/jobs/{id}/download-url` gera URL nova enquanto o objeto temporário existir. |
| **Prefixo reservado `_backup-tmp/`** | Bloco de chaves do S3 dedicado a artefatos transitórios (ZIPs de backup e restore). `AttachmentStore.CreateFile` rejeita explicitamente `subdir` igual a `_backup-tmp` ou começando com `_backup-tmp/` — defesa em profundidade contra colisão acidental ou injeção de path por código futuro. |
| **Limpeza pós-crash** | No startup, se backend ativo for S3, goroutine não-bloqueante remove qualquer objeto em `_backup-tmp/` com idade > 24h via `s3.DeleteObjects` batch. Cobre cenário onde a aplicação cai durante backup/restore antes do cleanup inline executar. |
| **Verificação de integridade** | Restauração computa SHA256 de cada entrada do ZIP e compara contra o nome da entrada (que é o hash declarado). Mismatch é contado em `hash_mismatch` e listado em `skipped` — backend de destino **não** recebe escrita para entrada inválida. Backup também verifica via `io.TeeReader` durante a composição do ZIP. |
| **Entradas órfãs** | Restauração só escreve no backend quando há linha em `attachments` com `content_sha256` correspondente (`SELECT ... WHERE content_sha256 = ?` usa `idx_attachments_content_sha256`). Entradas órfãs no ZIP são ignoradas e listadas — restauração **nunca cria linhas novas em `attachments`**. |
| **Concorrência** | `BackupJobManager` mantém no máximo 1 job ativo por backend (`local` ou `s3`). Tentativa concorrente recebe `ErrJobInFlight` → HTTP 409. Mutex bloqueia race entre Start/Get/Finish. |
| **Recuperação de panic** | Goroutines worker (`runBackupJob`, `runRestoreJob`) têm `recover()` que finaliza o job como `failed` com mensagem `panic: <r>` e dispara cleanup do temp object — panic em um job não derruba o processo. |
| **Erros sanitizados** | `sanitizeAWSError` em `internal/storage/s3.go` redige strings que contenham "AccessKey" ou "SecretKey" antes de retornar a mensagem ao cliente. |

**IAM mínimo para o backend S3**:

```json
{"Statement":[{
  "Effect":"Allow",
  "Action":["s3:GetObject","s3:PutObject","s3:DeleteObject",
            "s3:ListBucket","s3:DeleteObjects","s3:AbortMultipartUpload"],
  "Resource":["arn:aws:s3:::BUCKET","arn:aws:s3:::BUCKET/*"]
}]}
```

`PresignGetObject` é client-side e não requer permissão IAM adicional além de `s3:GetObject` (usado pelo destinatário via URL assinada).

---

## O que o PKD não protege

| Ameaça | Não protegido |
|---|---|
| Comprometimento do SO | Acesso root ao host → leitura direta do arquivo SQLite. |
| Acesso físico | Idem. |
| Brute force de IDs de sessão | 32 bytes aleatórios = 256 bits de entropia. Não factível, mas sem rate limit em session lookup (apenas no login). |
| Timing attacks em session lookup | Map lookup não é constant-time. Aceitável pela alta entropia dos IDs. |
| Confusão de MIME em anexos | O MIME armazenado é o que o uploader enviou. Downloads forçam `Content-Disposition: attachment` para prevenir execução inline. |
| Conteúdo externo malicioso via captura | Open Graph extraction faz HTTP GET a sites externos. O conteúdo retornado passa por sanitização, mas um site externo comprometido poderia tentar explorar o parser HTML. |
