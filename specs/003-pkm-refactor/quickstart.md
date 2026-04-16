# Quickstart: PKD v2 — Personal Knowledge Management (003-pkm-refactor)

**Feature**: `003-pkm-refactor`
**Goal**: Instalar e começar a usar o PKD v2 com links bidirecionais, graph view e captura de conteúdo externo.

> Todos os valores neste documento são **placeholders**. Nunca use senhas de exemplo em produção.

---

## 1. Pré-requisitos

| Necessário | Por quê | Como verificar |
|---|---|---|
| Docker 24+ (ou UNRAID 6.12+) | A app é entregue como container | `docker --version` |
| Diretório para o SQLite | O banco fica fora do container | Criar `/path/to/pkd/db/` |
| Diretório para anexos | Arquivos ficam fora do container | Criar `/path/to/pkd/attachments/` |
| Senha mestra forte | Protege todo o conteúdo | `openssl rand -base64 24` |

---

## 2. docker run

```bash
mkdir -p /path/to/pkd/db /path/to/pkd/attachments

docker run -d \
  --name pkd \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /path/to/pkd/db:/data/db \
  -v /path/to/pkd/attachments:/data/attachments \
  -e PKD_PASSWORD='SUBSTITUA_POR_SENHA_FORTE' \
  -e PKD_DB_PATH=/data/db/pkd.sqlite \
  -e PKD_ATTACHMENTS_PATH=/data/attachments \
  -e PKD_BASE_URL='https://pkd.dalc.in/' \
  ghcr.io/edalcin/pkd:latest
```

Abra `http://localhost:8080`, digite a senha.

---

## 3. UNRAID

Veja o documento **[UNRAID.md](../../UNRAID.md)** na raiz do repositório para instruções completas em português via interface gráfica.

---

## 4. Funcionalidades novas no v2

### Links bidirecionais (`[[nome do documento]]`)

No editor TipTap, digite `[[` para abrir o autocompletar de documentos. Selecione o documento alvo — um link bidirecional é criado automaticamente. O documento alvo mostrará o seu na seção "Referenciado por".

### Graph View

Acesse pela barra lateral ou menu. O grafo mostra documentos como nós e links como arestas. Apenas documentos com ≥1 link aparecem por padrão. Interação: zoom (scroll), pan (arrastar fundo), clique em nó abre o documento. Nós coloridos por tag primária.

### Captura de conteúdo externo

**No celular**: Compartilhe um link, texto ou imagem de qualquer app para o PKD (via menu "Compartilhar" do SO → PKD). Um novo documento é criado com tag `#captura`.

**Via API**: POST autenticado para `/api/capture`:
```bash
curl -X POST http://localhost:8080/api/capture \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: <token>" \
  -H "Cookie: pkd_session=<session>" \
  -b "pkd_csrf=<csrf>" \
  -d '{"title": "Artigo interessante", "url": "https://example.com", "tags": ["leitura"]}'
```

---

## 5. Variáveis de ambiente

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `PKD_PASSWORD` | **sim** | — | Senha mestra |
| `PKD_DB_PATH` | **sim** | — | Caminho do arquivo SQLite |
| `PKD_ATTACHMENTS_PATH` | **sim** | — | Caminho do diretório de anexos |
| `PKD_LISTEN_ADDR` | não | `:8080` | Endereço de escuta |
| `PKD_SESSION_IDLE_MINUTES` | não | `60` | Timeout de sessão inativa |
| `PKD_MAX_IMAGE_MB` | não | `10` | Tamanho máximo de imagem inline |
| `PKD_MAX_ATTACHMENT_MB` | não | `100` | Tamanho máximo de anexo |
| `PKD_TRUST_PROXY_HEADERS` | não | `0` | `1` apenas atrás de proxy reverso confiável |
| `PKD_BASE_URL` | não | *(host da req.)* | URL pública base para links compartilhados (ex: `https://pkd.dalc.in/`) |

---

## 6. Smoke test (v2)

Após instalar, verifique em ordem:

1. **Criar documento** — clique `+`, dê título, digite conteúdo. Recarregue — persiste.
2. **Link bidirecional** — abra um documento, digite `[[` e selecione outro. Abra o outro — veja "Referenciado por".
3. **Graph View** — abra o grafo. Os dois documentos linkados aparecem como nós conectados.
4. **Busca** — busque por substring de título e corpo. Resultados com snippet.
5. **Tags** — adicione tags. Filtre a árvore por tag.
6. **Captura** — no celular, compartilhe um link com o PKD (requer PWA instalado).
7. **Compartilhar** — gere link público. Abra em janela privada. Revogue. Confirme 404.
8. **Backup/Restore** — faça backup, altere algo, restaure.
9. **Tema** — alterne claro/escuro. Persiste após reload.
10. **Mobile** — abra no celular. Tudo funcional com touch.

---

## 7. Compilar do código-fonte

```bash
git clone https://github.com/edalcin/pkd.git
cd pkd

# Frontend (Svelte)
cd frontend && npm install && npm run build && cd ..

# Backend (Go)
go test ./...
PKD_PASSWORD=devpassword \
PKD_DB_PATH=/tmp/pkd.sqlite \
PKD_ATTACHMENTS_PATH=/tmp/pkd-att \
go run ./cmd/pkd
```

Requer Go 1.25+ e Node.js 20+.

---

## 8. Documentação C4 Model

A arquitetura do sistema está documentada em 4 níveis no diretório `docs/c4/`:

- **Context** (`context.md`) — Visão geral: usuário, sistema, integrações externas
- **Container** (`container.md`) — Backend Go, frontend Svelte, SQLite, volumes
- **Component** (`component.md`) — Módulos internos: server, store, security, sessions
- **Code** (`code.md`) — Detalhes de implementação: structs, handlers, stores

Todos os diagramas são em Mermaid e renderizam nativamente no GitHub.
