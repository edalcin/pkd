# PKD no UNRAID — Guia de Instalação

Este guia mostra como instalar o PKD no UNRAID usando a interface gráfica **Docker → Add Container**. Nenhum terminal é necessário.

> **Imagem Docker:** `ghcr.io/edalcin/pkd:latest`

---

## Pré-requisitos

- UNRAID 6.12 ou superior
- Serviço Docker ativo
- Acesso à internet para baixar a imagem (ou transfira a imagem previamente em uma máquina com internet)

---

## Passo 1 — Criar os diretórios no host

Abra o gerenciador de arquivos do UNRAID (aba **Files**) e crie dois diretórios:

| Finalidade | Caminho sugerido |
|---|---|
| Banco de dados SQLite | `/mnt/user/appdata/pkd/db` |
| Arquivos anexados | `/mnt/user/appdata/pkd/attachments` |

Os dois diretórios devem ter permissão de escrita para o usuário não-root do container (UID/GID 65532). O UNRAID normalmente cria diretórios com permissões adequadas por padrão.

> **Dica:** Você também pode criar os diretórios pelo terminal do UNRAID:
> ```bash
> mkdir -p /mnt/user/appdata/pkd/db /mnt/user/appdata/pkd/attachments
> ```

---

## Passo 2 — Abrir Docker → Add Container

Na barra superior do UNRAID, vá em **Docker → Add Container**.

---

## Passo 3 — Preencher as configurações básicas do container

| Campo | Valor |
|---|---|
| **Name** | `pkd` |
| **Repository** | `ghcr.io/edalcin/pkd:latest` |
| **Network Type** | `Bridge` |
| **WebUI** | `http://[IP]:[PORT:8080]` |

---

## Passo 4 — Adicionar mapeamento de porta

Clique em **Add another Path, Port, Variable, Label or Device** → **Port**.

| Campo | Valor |
|---|---|
| Name | `WebUI` |
| Container Port | `8080` |
| Host Port | `8080` *(altere se a porta 8080 já estiver em uso)* |
| Connection Type | `TCP` |

---

## Passo 5 — Adicionar volume do banco de dados

Clique em **Add another Path, Port, Variable, Label or Device** → **Path**.

| Campo | Valor |
|---|---|
| Name | `DB` |
| Container Path | `/data/db` |
| Host Path | `/mnt/user/appdata/pkd/db` |
| Access Mode | `Read/Write` |

> ⚠️ **O caminho do host deve começar com `/`** (barra inicial obrigatória).  
> Exemplo correto: `/mnt/user/appdata/pkd/db`  
> Exemplo errado: `mnt/user/appdata/pkd/db`  
> Sem a barra inicial, o Docker interpreta o valor como nome de volume interno e falha com erro de caracteres inválidos.

---

## Passo 6 — Adicionar volume de anexos

Clique em **Add another Path, Port, Variable, Label or Device** → **Path**.

| Campo | Valor |
|---|---|
| Name | `Attachments` |
| Container Path | `/data/attachments` |
| Host Path | `/mnt/user/appdata/pkd/attachments` |
| Access Mode | `Read/Write` |

---

## Passo 7 — Adicionar variável: Senha mestra

Clique em **Add another Path, Port, Variable, Label or Device** → **Variable**.

| Campo | Valor |
|---|---|
| Name | `Master Password` |
| Key | `PKD_PASSWORD` |
| Value | *(gere com `openssl rand -base64 24` ou use uma senha forte de sua escolha)* |
| Type | `Password` *(o UNRAID oculta o campo automaticamente)* |

> ⚠️ **Não reutilize senhas existentes.** Esta é a única proteção de acesso ao seu banco de conhecimento.

---

## Passo 8 — Adicionar variável: Caminho do banco de dados

Clique em **Add another Path, Port, Variable, Label or Device** → **Variable**.

| Campo | Valor |
|---|---|
| Key | `PKD_DB_PATH` |
| Value | `/data/db/pkd.sqlite` |

---

## Passo 9 — Adicionar variável: Caminho dos anexos

Clique em **Add another Path, Port, Variable, Label or Device** → **Variable**.

| Campo | Valor |
|---|---|
| Key | `PKD_ATTACHMENTS_PATH` |
| Value | `/data/attachments` |

---

## Passo 10 — Aplicar

Clique em **Apply**. O UNRAID baixa a imagem, cria o container e o inicia. O log do container exibirá:

```
listening on :8080
schema ready at /data/db/pkd.sqlite
```

Clique no link **WebUI** que aparece no painel do container. Digite a senha mestra cadastrada no Passo 7. Pronto — você está dentro!

---

## Variáveis de ambiente opcionais

Você pode adicionar estas variáveis para personalizar o comportamento:

| Variável | Padrão | Descrição |
|---|---|---|
| `PKD_LISTEN_ADDR` | `:8080` | Endereço e porta de escuta HTTP |
| `PKD_SESSION_IDLE_MINUTES` | `60` | Minutos de inatividade até expirar a sessão |
| `PKD_MAX_IMAGE_MB` | `10` | Tamanho máximo de imagem inline (MB) |
| `PKD_MAX_ATTACHMENT_MB` | `100` | Tamanho máximo de arquivo anexado (MB) |
| `PKD_TRUST_PROXY_HEADERS` | `0` | Defina como `1` apenas quando há um proxy reverso confiável na frente |

---

## Colocar o PKD atrás de HTTPS (recomendado)

Os links de compartilhamento geram URLs públicas. Essas URLs devem usar HTTPS. O UNRAID tem o plugin **SWAG** (Secure Web Application Gateway) disponível no Community Applications — é a forma mais simples de configurar um proxy reverso com certificado SSL automático.

Após configurar o SWAG, adicione um arquivo de configuração de subdomínio que encaminhe para o container `pkd` na porta `8080`.

Quando estiver atrás de um proxy reverso, adicione também a variável:

| Key | `PKD_TRUST_PROXY_HEADERS` |
|---|---|
| Value | `1` |

> ⚠️ **Só ative esta opção com um proxy reverso confiável na frente.** Sem isso, qualquer pessoa pode falsificar o IP de origem e contornar o bloqueio por tentativas de login.

---

## Atualizar para uma nova versão

1. No painel Docker do UNRAID, clique no ícone do container `pkd` → **Force Update**.
2. O UNRAID baixa a nova imagem e reinicia o container.
3. Seus dados nos volumes montados não são afetados pela atualização.

---

## Backup dos dados

Os dados ficam em dois lugares fora do container:

| O que | Onde no host |
|---|---|
| Banco de dados | `/mnt/user/appdata/pkd/db/pkd.sqlite` |
| Arquivos anexados | `/mnt/user/appdata/pkd/attachments/` |

**Backup pelo próprio PKD:** faça login → **Administração → Download backup**. O browser baixa um snapshot consistente do banco (via `VACUUM INTO`).

**Backup manual:** copie os dois diretórios acima para um local seguro. Recomenda-se incluir esse caminho na solução de backup do UNRAID (ex: Appdata Backup plugin).

---

## Solução de problemas

| Sintoma | Causa provável | Solução |
|---|---|---|
| Container para imediatamente após iniciar | Variável de ambiente obrigatória ausente | Verifique se `PKD_PASSWORD`, `PKD_DB_PATH` e `PKD_ATTACHMENTS_PATH` estão definidas |
| `permission denied` nos logs | Diretório do host sem permissão de escrita para UID 65532 | Execute no terminal do UNRAID: `chmod 777 /mnt/user/appdata/pkd/db /mnt/user/appdata/pkd/attachments` |
| Página de login abre mas o login falha | Senha incorreta ou com espaços extras | Verifique o valor da variável `PKD_PASSWORD`; sem espaços antes ou depois |
| Erro 502 vindo do SWAG | Porta errada na configuração do SWAG | Certifique-se de que o SWAG aponta para `pkd:8080`, não `localhost:8080` |
| Imagem não carrega no editor | Arquivo muito grande | Verifique o valor de `PKD_MAX_IMAGE_MB` (padrão: 10 MB) |
| `invalid characters for a local volume name` | Caminho do host sem `/` inicial | O campo **Host Path** deve começar com `/`, ex: `/mnt/user/appdata/pkd/db` |

---

## Logs do container

Para verificar o que acontece dentro do container:

1. No painel Docker do UNRAID, clique no ícone do container `pkd`.
2. Selecione **Logs**.

Ou pelo terminal:

```bash
docker logs pkd
docker logs pkd --follow  # acompanhar em tempo real
```

---

## Saúde do serviço

O PKD expõe um endpoint de healthcheck:

```
GET http://<seu-servidor>:8080/healthz
```

Retorna `{"status":"ok"}` quando o banco de dados está acessível. Use com ferramentas de monitoramento como **Uptime Kuma** (disponível no Community Applications do UNRAID).
