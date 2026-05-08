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

## Limpeza de órfãos

Com o tempo, documentos excluídos podem deixar arquivos de anexo órfãos no disco (arquivos sem linha correspondente na tabela `attachments`). Para removê-los:

**Administração → Limpeza → Iniciar limpeza**

Isso:
1. Percorre o diretório de anexos.
2. Exclui arquivos sem linha na tabela `attachments`.
3. Executa `VACUUM` no banco para liberar espaço.

Seguro de executar a qualquer momento. Exibe quantos arquivos foram removidos.

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

### UNRAID (dev — tag `:edge`)

O UNRAID usa `:edge`, que atualiza automaticamente a cada push em `main`.

1. Docker tab → clique no ícone do container `pkd` → **Force Update**.
2. O UNRAID baixa a nova imagem `:edge` e reinicia o container.
3. Seus dados em `/mnt/user/appdata/pkd/` ficam intocados.

Ou com Watchtower (atualização automática sem intervenção manual).

### EC2 (produção — tag `:stable`)

A EC2 usa `:stable`, que **só muda quando você promove manualmente**. Veja §"Promoção dev → prod" abaixo.

```bash
docker pull ghcr.io/edalcin/pkd:stable
docker stop pkd && docker rm pkd
# Re-execute o comando docker run original (dados ficam nos volumes)
```

---

## Promoção dev → prod

Depois de validar a versão rodando no UNRAID:

```bash
# 1. Verificar qual commit está rodando em dev
docker inspect ghcr.io/edalcin/pkd:edge --format '{{index .RepoDigests 0}}'

# 2. Criar tag Git semver no commit atual de main
git tag -a v1.2.3 -m "Release 1.2.3

- feat: armazenamento de anexos em S3
- fix: descrição do fix
"
git push origin v1.2.3
```

O workflow `promote-to-prod.yml` dispara automaticamente, re-tagueando a imagem `:sha-*` correspondente como `:v1.2.3` e `:stable` (sem rebuild).

```bash
# 3. Na EC2, puxar a nova :stable e reiniciar
docker pull ghcr.io/edalcin/pkd:stable
docker stop pkd && docker rm pkd
# Re-execute o comando docker run original
```

### Reversão de produção

```bash
# Listar versões disponíveis
docker images ghcr.io/edalcin/pkd

# Reiniciar com versão anterior
docker pull ghcr.io/edalcin/pkd:v1.2.2
docker stop pkd && docker rm pkd
docker run ... ghcr.io/edalcin/pkd:v1.2.2
```

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
