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

## O que o PKD não protege

| Ameaça | Não protegido |
|---|---|
| Comprometimento do SO | Acesso root ao host → leitura direta do arquivo SQLite. |
| Acesso físico | Idem. |
| Brute force de IDs de sessão | 32 bytes aleatórios = 256 bits de entropia. Não factível, mas sem rate limit em session lookup (apenas no login). |
| Timing attacks em session lookup | Map lookup não é constant-time. Aceitável pela alta entropia dos IDs. |
| Confusão de MIME em anexos | O MIME armazenado é o que o uploader enviou. Downloads forçam `Content-Disposition: attachment` para prevenir execução inline. |
| Conteúdo externo malicioso via captura | Open Graph extraction faz HTTP GET a sites externos. O conteúdo retornado passa por sanitização, mas um site externo comprometido poderia tentar explorar o parser HTML. |
