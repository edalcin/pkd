# PKD Operations Guide

Manutenção do dia a dia, backup/restore e procedimentos de atualização.

---

## Backup

O PKD **não faz backups automáticos**. Você deve iniciá-los manualmente.

### Backup pelo app (recomendado)

1. Faça login no PKD.
2. Vá em **Administração → Backup**.
3. Clique em **Download backup**. O browser baixa `pkd-backup-YYYY-MM-DD.sqlite`.

Usa SQLite `VACUUM INTO`, que gera um snapshot consistente e defragmentado mesmo com escrita em progresso (modo WAL). O arquivo baixado é um banco SQLite válido e completo.

**Guarde o arquivo em local seguro** — HD externo, cloud storage, segundo servidor. O PKD não envia backups a lugar algum.

### Backup no nível do host (alternativa)

```bash
cp /mnt/user/appdata/pkd/db/pkd.sqlite /mnt/user/backups/pkd-$(date +%Y%m%d).sqlite
rsync -a /mnt/user/appdata/pkd/attachments/ /mnt/user/backups/pkd-attachments/
```

Funciona com WAL mas é ligeiramente menos consistente que o backup pelo app. Sempre inclua o diretório de anexos.

### Frequência sugerida

- Diário → manter 7 dias.
- Semanal → manter 4 semanas.
- Mensal → manter indefinidamente.

---

## Restore

### Pelo app

1. Faça login.
2. **Administração → Restaurar** → selecione o arquivo `.sqlite`.
3. Confirme com `REPLACE`.
4. Clique em **Restaurar**. O servidor troca o arquivo e solicita novo login.

### Manual (nível do host)

1. Pare o container: `docker stop pkd`.
2. Substitua o arquivo: `cp /path/to/backup.sqlite /mnt/user/appdata/pkd/db/pkd.sqlite`.
3. Inicie o container: `docker start pkd`.

---

## Backup e restauração de anexos (S3)

Quando o backend ativo é S3, o PKD oferece backup/restauração assíncrona de arquivos anexados sem materializar o ZIP no disco da instância de aplicação.

### Backup assíncrono

1. **Administração → Storage → "Backup assíncrono (S3)"** (visível só com backend S3 ativo).
2. Clique em **"☁ Iniciar backup S3"**. Um job assíncrono é criado.
3. Polling automático mostra progresso `X / Y anexos`.
4. Ao concluir, clique em **"⬇ Baixar ZIP"**. URL pré-assinada com **TTL de 15 minutos**.
5. Se a URL expirar, clique em **"Gerar nova URL"** (enquanto o ZIP temporário existir no S3).

**O que está dentro do ZIP**:
- Uma entrada por SHA256 único (anexos com mesmo conteúdo são deduplicados).
- `manifest.json` (última entrada) mapeia cada SHA256 → lista de `stored_filename`s + tamanho + MIME.

**Convenção de nome**:
ZIP temporário fica em `<prefix>/_backup-tmp/<job-id>.zip`. Este prefixo é **reservado** — nenhum anexo real usa essa rota.

### Restauração assíncrona

1. **Administração → Storage → "Restauração assíncrona"** (visível para qualquer backend ativo).
2. Selecione o ZIP gerado por esta aplicação.
3. Escolha o comportamento se a chave de destino já existir:
   - **Sobrescrever** (padrão) — substitui pelo conteúdo do ZIP.
   - **Manter existente** — pula entradas que já existem.
   - **Abortar na primeira colisão** — interrompe a operação.
4. Clique em **"⬆ Iniciar restauração"**.
5. Acompanhe os contadores: `gravados / mantidos / órfãos ignorados / hash inválido`.
6. Entradas ignoradas (órfãs ou com hash inválido) aparecem em lista colapsável.

**Cross-backend**: ZIP gerado em produção (backend S3) pode ser restaurado em desenvolvimento (backend local) e vice-versa. O manifesto é backend-agnóstico — usa apenas SHA256.

**Órfãs**: entradas do ZIP cuja SHA256 não corresponde a nenhuma linha de `attachments` na base atual são **ignoradas** (não escritas no backend). São listadas no log da operação. Restauração nunca cria linhas novas em `attachments` — só reidrata arquivos para linhas existentes.

### IAM mínimo (EC2 / role / usuário)

```json
{
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "s3:GetObject", "s3:PutObject", "s3:DeleteObject",
      "s3:ListBucket", "s3:DeleteObjects",
      "s3:AbortMultipartUpload"
    ],
    "Resource": [
      "arn:aws:s3:::SEU-BUCKET",
      "arn:aws:s3:::SEU-BUCKET/*"
    ]
  }]
}
```

- `s3:DeleteObjects` (batch) é necessário para o sweep de órfãos temporários no startup.
- `s3:AbortMultipartUpload` cobre uploads multipart abortados (backup ou restore).
- `PresignGetObject` é client-side; o destinatário usa `s3:GetObject` via URL assinada.

### Limpeza automática (sweep)

A cada startup, se o backend ativo for S3, a aplicação:
1. Lista objetos sob `<prefix>/_backup-tmp/`.
2. Remove qualquer objeto com idade **> 24h**.
3. Loga `backup sweep: removed N stale temp object(s)`.

Cobre crashes da aplicação que deixaram ZIPs intermediários órfãos.

### Concorrência

Uma operação ativa por backend por vez. Tentativa de iniciar segunda operação retorna **409 Conflict** com mensagem "Já existe uma operação em andamento para este backend".

### Migração, reconciliação e limpeza (operações assíncronas)

Todas as operações abaixo são **assíncronas**: ao clicar o botão é criado um job em background; a UI exibe barra de progresso (`X / Y arquivos`) e relatório final ao concluir. Somente uma operação pode estar ativa por backend por vez — uma segunda tentativa retorna **409 Conflict**.

#### Migrar para backend ativo

**Administração → Storage → "📦 Migrar para backend ativo"** (visível com S3 configurado)

Copia todos os anexos cujo `storage_location` difere do backend ativo para o backend ativo, verificando SHA256 de origem e destino. Não-destrutivo: os arquivos da origem **não são removidos**. Idempotente — pode ser repetido com segurança após falha.

Relatório ao concluir: `total_found / copiados / erros`.

Se `total_found = 0` mas você sabe que há arquivos no outro backend, execute primeiro **Reconciliar** para corrigir `storage_location` no banco.

#### Reconciliar S3 ↔ DB / LOCAL ↔ DB

Corrige o campo `storage_location` no banco de dados quando ele está desatualizado em relação ao que existe fisicamente nos backends. Não move arquivos — apenas corrige registros.

| Botão | Quando usar |
|---|---|
| **🔄 Reconciliar S3 ↔ DB** | Após restore do DB sem os arquivos S3 correspondentes (o banco diz `local` mas o arquivo está no S3). |
| **🔄 Reconciliar LOCAL ↔ DB** | Após restore do DB sem os arquivos locais correspondentes (o banco diz `s3` mas o arquivo está no disco). Sempre visível, mesmo sem S3 configurado. |

Fluxo: varre o backend selecionado, compara chaves com `stored_filename` na tabela `attachments` e corrige `storage_location`. Orphans (chaves no backend sem linha no DB) são ignorados.

Relatório ao concluir: `registros corrigidos / erros`. Se corrigidos > 0, execute **Migrar** em seguida para copiar os arquivos para o backend ativo.

#### Limpar origem

**Administração → Storage → "🗑 Limpar origem"** (visível com S3 configurado)

Remove os arquivos do backend de **origem** (o que não é o ativo) para os anexos já migrados. Antes de excluir cada arquivo, **verifica se ele realmente existe no backend ativo** via `Get`. Arquivos sem cópia confirmada no destino são **ignorados** (contados como `skipped`) — não são apagados para evitar perda de dados.

Relatório ao concluir: `candidatos / removidos / ignorados (sem cópia no destino) / erros`.

Se `skipped > 0`: execute **Migrar** novamente para garantir que esses arquivos sejam copiados antes de tentar limpar novamente.

**Ordem recomendada de uso**:
1. Migrar → aguardar `copiados = total_found`, sem erros
2. Limpar origem → confirmar `ignorados = 0` antes de considerar concluído

---

## Histórico de versões

O PKD snapshota o conteúdo (título + corpo + ícone) de cada documento a cada save.

- **Dedup por SHA-256** — saves sem mudança de conteúdo não geram nova versão.
- **Retenção padrão** — 50 versões por documento. Configurável em **Administração → Configurações → Máx. versões por documento**.
- **Visualizar** — botão `⏱` na barra de ações do documento.
- **Restaurar** — clique em "Restaurar esta versão" no dialog. Usa o fluxo normal de save (bloqueio otimista; retorna `409` em caso de conflito de versão).
- **Excluir versão** — ícone de lixeira por entrada (hover-reveal), com confirmação.

Versões são excluídas em cascata quando o documento é excluído permanentemente.

---

## Limpeza de órfãos

Com o tempo, documentos excluídos ou uploads com falha podem deixar arquivos no disco sem linha correspondente na tabela `attachments`. O PKD oferece controle individual de cada arquivo órfão.

### Verificar e gerenciar

1. **Administração → Arquivos**.
2. Clique em **"🔍 Verificar Órfãos"** (ao lado de "↻ Atualizar").
3. A lista de arquivos órfãos aparece abaixo dos anexos normais, com nome e tamanho de cada arquivo.
4. Por arquivo:
   - **"⬇ Baixar"** — faz download do arquivo antes de decidir eliminar.
   - **"🗑 Eliminar"** — exclui o arquivo com confirmação. Irreversível.

### VACUUM do banco

Para recuperar espaço em disco no banco SQLite após exclusões:

**Administração → Limpeza → Iniciar VACUUM**

Executa `VACUUM` no banco. Não remove arquivos de anexo (use "Verificar Órfãos" para isso).

Seguro de executar a qualquer momento.

---

## Gerenciamento de hashtags

### Renomear ou mesclar uma tag

**Administração → Tags → Renomear**

- Preencha a tag atual e o novo nome → clique em **Renomear**.
- Se o novo nome **não existe**: a tag é renomeada. Todos os documentos com a tag antiga passam a ter o novo nome.
- Se o novo nome **já existe**: as duas tags são **mescladas**. Todos os documentos da tag antiga são reassignados para a tag existente. A tag antiga é excluída.

---

## Gestão da lixeira

Documentos excluídos vão para a Lixeira e ficam lá **indefinidamente** até você os remover.

- **Ver lixeira**: Administração → Lixeira.
- **Restaurar um**: clique em **Restaurar**. O documento retorna ao pai original (ou raiz se o pai também foi excluído).
- **Excluir permanentemente**: clique em **Excluir**. Irreversível.
- **Esvaziar tudo**: clique em **Esvaziar Tudo**. Irreversível.

---

## Links bidirecionais — manutenção

Links bidirecionais são sincronizados automaticamente com o corpo do documento a cada save. Não é necessária manutenção manual.

- **Links para documentos excluídos (trash)**: aparecem marcados como "quebrados" na UI. Os links permanecem no banco de dados.
- **Links para documentos permanentemente excluídos**: removidos automaticamente pelo `ON DELETE CASCADE`.
- **Documentos sem links**: não aparecem no Graph View por padrão (toggle "Mostrar todos" para incluí-los).

---

## Graph View

O grafo é renderizado client-side com D3.js. Para grandes bases de dados (500+ documentos conectados), pode levar 1-2 segundos para a simulação force-directed convergir. Não há cache server-side do grafo — ele é recalculado a cada abertura.

---

## Invalidação de cache PWA

O service worker (`sw.js`) versiona o cache do app shell. Na versão v2:
- Cache shell: `pkd-shell-v5`
- Cache de documentos: `pkd-docs-v5`

Após uma atualização do PKD, o handler `activate` do SW remove automaticamente caches com outros nomes. Se o navegador continuar servindo a versão antiga:

1. Abra DevTools → Application → Service Workers.
2. Clique em **Unregister**.
3. Recarregue a página.

---

## Links públicos de compartilhamento

Links públicos são gerados com base em `PKD_BASE_URL`. Configure esta variável para o domínio público onde o PKD está disponível:

```
PKD_BASE_URL=https://pkd.dalc.in/
```

Se `PKD_BASE_URL` não estiver definida, o servidor usa o host da requisição HTTP (funciona para acesso direto, mas pode falhar atrás de proxy reverso que altera o Host header).

---

## Atualizações

### Modelo de tags Docker

| Tag | Semântica | Ambiente |
|---|---|---|
| `:edge` | Sempre aponta para o último commit de `main` | UNRAID (dev) |
| `:sha-abc1234` | Imutável — identifica um commit específico | Referência de auditoria |
| `:stable` | Aponta para a versão atual em produção | EC2 (prod) |
| `:v1.2.3` | Imutável — release semver | Produção, histórico |

### UNRAID (dev/homologação — tag `:edge`)

O UNRAID usa `:edge`, que é publicada a cada push em `main`. O container
chama-se `pkd2` (`https://pkd2.dalc.in`); os dados ficam em
`/mnt/user/Storage/appsdata/pkd/` e os anexos no bucket `pkd-dev-attachments`
(`attachments.backend = s3` no banco).

1. Docker tab → clique no ícone do container `pkd2` → **Force Update**
   (ou, por SSH: `/usr/local/emhttp/plugins/dynamix.docker.manager/scripts/update_container pkd2`
   e, se ele estiver parado, `docker start pkd2`).
2. O UNRAID baixa a nova imagem `:edge` e recria o container.
3. Os dados ficam intocados.

Teste primeiro no Docker local (Windows); o UNRAID é o ambiente de homologação.

### EC2 (produção — tag `:stable`, fixada por digest)

A EC2 roda o PKD pelo `/home/ec2-user/docker-compose.yml` (serviços
`cloudflare`, `notas`, `spl`, `web`, `pkd`). A linha `image:` do `pkd` fixa o
**digest** da `:stable` promovida, com o comentário da versão, por exemplo:

```yaml
image: ghcr.io/edalcin/pkd@sha256:<digest>  # tag de origem: stable (v1.3.0)
```

Os anexos de produção ficam no bucket `pkd-prod-attachments`. O container não
publica porta: o acesso é pelo Cloudflare Tunnel (`pkd.dalc.in`, protegido por
Cloudflare Access). Para chamar a API a partir de fora, use um túnel SSH até o
IP do container na rede `cloudflare`.

> ⚠️ Rode sempre `docker compose up -d pkd`, **com o nome do serviço**. Sem o
> nome, o compose também liga o `notas`, que foi desligado de propósito
> (`docker stop notas`, 2026-09-26) e deve continuar parado.

---

## Promoção dev → prod

Depois de validar a versão rodando no UNRAID:

```bash
# 1. Verificar qual commit está rodando em dev
docker image inspect ghcr.io/edalcin/pkd:edge --format '{{index .Config.Labels "org.opencontainers.image.revision"}}'

# 2. Criar tag Git semver nesse commit
git tag -a v1.2.3 <commit> -m "Release 1.2.3

- feat: …
- fix: …
"
git push origin v1.2.3
```

O workflow `promote-to-prod.yml` dispara automaticamente, re-tagueando a imagem `:sha-*` correspondente como `:v1.2.3` e `:stable` (sem rebuild).

```bash
# 3. Na EC2: backup antes de atualizar
B=/home/ec2-user/pkd-backups/$(date +%F)-pre-vX.Y.Z; mkdir -p $B
sqlite3 /home/ec2-user/pkd/db/pkd.sqlite ".backup $B/pkd.sqlite"   # cópia consistente com o PKD ligado
cp -p /home/ec2-user/docker-compose.yml $B/
aws s3 sync s3://pkd-prod-attachments $B/s3-pkd-prod-attachments

# 4. Puxar a nova :stable e anotar o digest
docker pull ghcr.io/edalcin/pkd:stable
docker image inspect ghcr.io/edalcin/pkd:stable --format '{{index .RepoDigests 0}}'

# 5. Trocar o digest na linha image: do pkd no docker-compose.yml e recriar SÓ o pkd
docker compose up -d pkd
```

### Reversão de produção

Volte a linha `image:` do `docker-compose.yml` salvo no backup (ou o digest da
versão anterior) e rode `docker compose up -d pkd`. Se a versão nova migrou o
banco de forma incompatível, pare o `pkd`, restaure o `pkd.sqlite` do backup
e suba de novo. As migrações do PKD até hoje são só aditivas (colunas e
índices novos), então a imagem anterior lê o banco novo.

Tempo total: **menos de 1 minuto**, sem rebuild, sem mexer em código.

### Migrações de schema

O PKD executa todo o DDL (`CREATE TABLE IF NOT EXISTS`) a cada inicialização. É seguro atualizar — o schema é sempre aplicado de forma idempotente. Não há arquivos de migração versionados para gerenciar. Novas tabelas (como `document_links` em v2) são criadas automaticamente no primeiro start após a atualização.

---

## Logs

- **Docker**: `docker logs pkd` ou `docker logs pkd --follow`.
- **UNRAID**: Docker tab → clique no container → **Logs**.
- O PKD escreve apenas em stdout/stderr.

---

## Monitoramento

Endpoint de healthcheck:

```
GET /healthz
```

Retorna `200 {"status":"ok"}` quando o banco está acessível, `503` caso contrário. Use com Uptime Kuma, Healthchecks.io, etc.
