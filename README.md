# PKD — Personal Knowledge Database

Base de conhecimento pessoal, auto-hospedada, entregue como uma única imagem Docker pequena (~20 MB).

**Imagem:** `ghcr.io/edalcin/pkd:latest`

---

## Funcionalidades

| | |
|---|---|
| 📁 **Hierarquia ilimitada** | Documentos dentro de documentos, arrastar e soltar para reorganizar |
| ✏️ **Editor rico** | CKEditor 5 com imagens inline redimensionáveis, tabelas, blocos de código |
| 🏷️ **Hashtags** | Marque documentos com `#tags` e filtre a árvore por uma ou mais tags |
| 🔍 **Super-busca** | Busca por substring em título, corpo e tags (SQLite FTS5) |
| 📅 **Calendário** | Navegue pelos documentos pela data de criação |
| 📎 **Anexos** | Arquivos ficam em um volume externo ao container, sobrevivem a atualizações |
| 🔗 **Links de compartilhamento** | Links públicos revogáveis, somente leitura, sem navegação |
| 🛡️ **Administração** | Backup/restore manual, limpeza, renomear/mesclar tags, lixeira |
| 🌙 **Tema claro/escuro** | Alternância persistida no `localStorage` |
| 📱 **Mobile-friendly** | Layout responsivo, alvos de toque ≥ 44 px |
| 📲 **PWA** | Instalável como app; modo offline somente leitura |

---

## Início rápido

```bash
docker run -d \
  --name pkd \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /caminho/para/pkd/db:/data/db \
  -v /caminho/para/pkd/attachments:/data/attachments \
  -e PKD_PASSWORD='SUBSTITUA_POR_UMA_SENHA_FORTE' \
  -e PKD_DB_PATH=/data/db/pkd.sqlite \
  -e PKD_ATTACHMENTS_PATH=/data/attachments \
  ghcr.io/edalcin/pkd:latest
```

Acesse `http://localhost:8080` e digite a senha mestra.

---

## Instalação no UNRAID

Veja o guia completo em português: **[UNRAID.md](UNRAID.md)**

Instalação via interface gráfica (Docker → Add Container), sem necessidade de terminal.

---

## docker compose

```yaml
services:
  pkd:
    image: ghcr.io/edalcin/pkd:latest
    container_name: pkd
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      PKD_PASSWORD: ${PKD_PASSWORD:?PKD_PASSWORD is required}
      PKD_DB_PATH: /data/db/pkd.sqlite
      PKD_ATTACHMENTS_PATH: /data/attachments
    volumes:
      - ./data/db:/data/db
      - ./data/attachments:/data/attachments
    healthcheck:
      test: ["/pkd", "-healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
```

```bash
export PKD_PASSWORD='SUBSTITUA_POR_UMA_SENHA_FORTE'
docker compose up -d
```

---

## Variáveis de ambiente

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `PKD_PASSWORD` | **sim** | — | Senha mestra (apenas em runtime, nunca armazenada) |
| `PKD_DB_PATH` | **sim** | — | Caminho do arquivo SQLite dentro do container |
| `PKD_ATTACHMENTS_PATH` | **sim** | — | Caminho do diretório de anexos dentro do container |
| `PKD_LISTEN_ADDR` | não | `:8080` | Endereço de escuta HTTP |
| `PKD_SESSION_IDLE_MINUTES` | não | `60` | Minutos de inatividade até expirar a sessão |
| `PKD_MAX_IMAGE_MB` | não | `10` | Tamanho máximo de imagem inline (MB) |
| `PKD_MAX_ATTACHMENT_MB` | não | `100` | Tamanho máximo de arquivo anexado (MB) |
| `PKD_TRUST_PROXY_HEADERS` | não | `0` | Defina como `1` apenas atrás de um proxy reverso confiável |

---

## Proxy reverso (HTTPS)

O PKD não gerencia certificados TLS — encerre o TLS em um proxy reverso (Caddy, Traefik, UNRAID SWAG).

Exemplo mínimo com **Caddy**:

```caddy
pkd.exemplo.lan {
    reverse_proxy localhost:8080
}
```

Ao usar proxy reverso, adicione `PKD_TRUST_PROXY_HEADERS=1` para que o bloqueio por tentativas de login use o IP real do cliente.

---

## Compilar a partir do código-fonte

Requer Go 1.25+. Sem CGO, sem Node.js.

```bash
git clone https://github.com/edalcin/pkd.git
cd pkd
go test ./...
PKD_PASSWORD=devpassword \
PKD_DB_PATH=/tmp/pkd.sqlite \
PKD_ATTACHMENTS_PATH=/tmp/pkd-att \
go run ./cmd/pkd
```

---

## Documentação

- [Guia de instalação UNRAID (PT-BR)](UNRAID.md)
- [Quickstart completo (EN)](specs/001-personal-knowledge-db/quickstart.md)
- [Referência de segurança (EN)](docs/security.md)
- [Guia de operações (EN)](docs/operations.md)

---

## Licença

MIT © 2026 Eduardo Dalcin
