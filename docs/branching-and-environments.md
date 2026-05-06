# Estratégia de Branches e Ambientes — PKD

**Status:** Documento de referência (didático). Não exige nenhuma mudança imediata.
**Data:** 2026-05-06
**Autor:** Claude (análise técnica), Eduardo Dalcin (decisão e revisão)
**Escopo:** Como organizar o repositório, o pipeline de build e o fluxo de promoção entre as duas instâncias do PKD agora que existe um ambiente de **desenvolvimento** (UNRAID) e um de **produção** (EC2 + S3).

> **Leitura prévia recomendada:** [`docs/s3-attachments-feasibility.md`](./s3-attachments-feasibility.md) — explica por que produção e desenvolvimento passam a ter custos, riscos e responsabilidades distintos.

---

## 1. Resumo executivo

| Decisão | Recomendação |
|---|---|
| Branches Git de longa duração | **Apenas `main`** (regra atual mantida) |
| Feature branches | Permitidas como **efêmeras**: nascem, viram PR, morrem ao merge |
| Imagem Docker | **Uma única imagem**, mesma `Dockerfile` para os dois ambientes |
| Distinção dev vs. prod | **Por *tag* da imagem Docker**, não por branch Git |
| UNRAID (dev) | Puxa `ghcr.io/edalcin/pkd:edge` — atualiza automaticamente a cada push em `main` |
| EC2 (prod) | Puxa `ghcr.io/edalcin/pkd:stable` (ou `:vX.Y.Z`) — atualizado **manualmente** após validar em dev |
| Promoção dev → prod | Re-tag da imagem (não merge, não rebuild) |
| Quando criar branch `production` | **Só** se surgir necessidade real de divergir código por dias/semanas (Plano B) |

**Em uma frase:** continue tratando `main` como única fonte da verdade; a separação entre dev e prod acontece **depois** do build, no nível da tag Docker, com promoção manual após validação no UNRAID.

---

## 2. Cenário atual

Hoje o pipeline (`.github/workflows/build-and-publish.yml`) faz:

1. Push em `main` → roda testes + `go vet` + `govulncheck`.
2. Builda a imagem Docker (single-stage frontend Node + Go + distroless).
3. Publica em `ghcr.io/edalcin/pkd:latest` e `ghcr.io/edalcin/pkd:sha-<curto>`.

Tanto **UNRAID** quanto **EC2** apontam para `:latest`. Resultado: qualquer commit em `main` chega aos dois ambientes praticamente ao mesmo tempo (segundos a minutos, dependendo da política de pull do Docker).

Esse modelo funcionou enquanto **só existia uma instância**, e enquanto **um bug em produção custava só um restart**.

---

## 3. O que muda com S3 + dois ambientes operacionais

| Fator | Antes | Agora |
|---|---|---|
| Bucket S3 | n/a | `pkd-prod-attachments` (prod) e `pkd-dev-attachments` (dev) — separados por env var |
| Auth AWS | n/a | IAM Role em prod (sem credencial); Access Key em dev |
| Custo de erro | Restart | Pode envolver dado em S3, latência paga em egress, alarme de Budget |
| Auditoria | Confiança no commit | Auditável por tag de imagem promovida + Git release |
| Janela de validação | Inexistente | Necessária — testar no UNRAID **antes** de promover para EC2 |

**Conclusão:** "tudo vai para os dois ao mesmo tempo" passa a ser arriscado. Precisa-se de uma janela onde a versão roda em dev e ainda não toca produção.

---

## 4. Princípio fundamental — separação **de código** vs. separação **de release**

Esta é a ideia central deste documento, e a mais comumente confundida.

### Separação de código (branch Git)

Existem **duas linhas de código diferentes** sendo mantidas em paralelo. Algo no código-fonte é diferente entre uma e outra: uma feature, um valor hardcoded, uma dependência. Manter duas linhas exige *merges*, *cherry-picks*, e cuidado constante para não divergirem além do esperado.

**Quando precisa:** quando a versão de prod e a de dev têm que ser **literalmente arquivos diferentes** (por exemplo, uma flag legal só ativada em prod que não pode ser feature-flag em runtime).

### Separação de release (tag de imagem Docker)

**Mesmo código, mesmo commit, mesmo binário.** O que difere é qual *snapshot já compilado* está rodando em cada ambiente. Promover prod = trocar a tag para a qual o Docker aponta. Reverter prod = apontar para a tag anterior. Não há merge, não há rebase, não há divergência possível.

**Quando precisa:** sempre que quero gating temporal (dev experimenta antes), mas o código é idêntico.

### Qual o caso do PKD?

**O segundo.** Diferenças entre dev e prod no PKD são **todas configuráveis em tempo de execução**:

- Path de attachments → `PKD_ATTACHMENTS_PATH`
- Backend de storage → tabela `settings` (ou env)
- Bucket S3 → `PKD_S3_BUCKET`, `PKD_S3_REGION`
- Credenciais → IAM Role (prod) ou env vars (dev)
- Base URL → `PKD_BASE_URL`

Não existe trecho do código que se compilaria diferente em dev vs. prod. Logo, **não há razão técnica para manter dois branches**.

---

## 5. Estratégia recomendada — "Trunk + tags promovidas"

### 5.1 Fluxo conceitual

```
                          (PR efêmero)
   feat/006-s3-storage  ────────────────►  main
                                            │
                                            │  push em main dispara CI
                                            ▼
                              ┌─────────────────────────────────┐
                              │  build-and-publish workflow     │
                              │                                 │
                              │  publica imagem com 2 tags:     │
                              │    :edge          (móvel)       │
                              │    :sha-abc1234   (imutável)    │
                              └─────────────────┬───────────────┘
                                                │
                                                ▼
                              ┌─────────────────────────────────┐
                              │  UNRAID puxa :edge              │
                              │  (auto, watchtower ou pull      │
                              │   periódico)                    │
                              └─────────────────┬───────────────┘
                                                │
                                                │  (você valida na UI)
                                                │
                                                ▼
                              ┌─────────────────────────────────┐
                              │  Decisão humana:                │
                              │  "está bom para promover?"      │
                              └─────────────────┬───────────────┘
                                                │
                                          git tag v1.2.3
                                          git push --tags
                                                │
                                                ▼
                              ┌─────────────────────────────────┐
                              │  promote-to-prod workflow       │
                              │                                 │
                              │  re-tagueia :sha-abc1234 como:  │
                              │    :v1.2.3                      │
                              │    :stable                      │
                              │  (sem rebuild)                  │
                              └─────────────────┬───────────────┘
                                                │
                                                ▼
                              ┌─────────────────────────────────┐
                              │  EC2 puxa :stable               │
                              │  (manual, após release tag)     │
                              └─────────────────────────────────┘
```

### 5.2 Glossário visual rápido

- **`main`** — único branch que vive para sempre. Toda mudança passa por ele.
- **feature branch (efêmera)** — `feat/<n>` ou `fix/<n>`. Aberta para isolar trabalho em curso, fechada no merge. **Não viola** "só main long-lived" porque dura horas/dias, não meses.
- **`:edge`** — tag Docker móvel que aponta sempre para o último commit de `main`.
- **`:sha-abc1234`** — tag Docker imutável, identifica um commit específico. Sempre puxa o mesmo binário.
- **`:stable`** — tag Docker móvel que aponta para a versão atualmente em produção.
- **`:v1.2.3`** — tag Docker imutável de release. Bate com a tag Git de mesmo nome.

### 5.3 Vantagens deste modelo para o PKD

| Vantagem | Como se manifesta |
|---|---|
| Honra `CLAUDE.md` ("só main long-lived") | Branches que aparecem são efêmeras, vivem dias |
| Zero overhead de merge entre branches | Só existe um branch persistente |
| Reversão de prod é instantânea | `docker pull pkd:v1.2.2 && restart` — sem rebuild |
| Auditoria clara | Cada release de prod tem tag Git + tag Docker imutável correspondente |
| S3 dev/prod naturalmente isolados | `PKD_S3_BUCKET` resolve via env, sem código diferente |
| CI continua simples | Mesmo Dockerfile, mesma stage de build |

### 5.4 Trade-offs honestos

- **Não permite divergir código entre ambientes.** Se um dia surgir necessidade real (ex: feature que só pode rodar em dev por restrição legal), use **feature flag em runtime**, não branch — adiciona-se uma flag em `settings` ou env e o código checa. Isso mantém o repositório enxuto.
- **Promoção é manual.** É um *feature*, não bug — é justamente esse momento humano que separa "passou nos testes" de "está pronto para usuário final". Documentar bem o checklist de validação reduz o risco de virar um ritual cego.
- **Esquecer de promover deixa prod desatualizada.** Mitigação: criar lembrete semanal/mensal e dashboard simples mostrando quantos commits `main` está à frente de `:stable`.

---

## 6. Fluxo de trabalho diário (referência prática)

### 6.1 Trabalho normal (commit que vai para dev imediatamente)

```bash
# Trabalho direto em main (caso comum, mudanças pequenas)
git pull
# … editar arquivos …
git add -A
git commit -m "feat(editor): adicionar suporte a X"
git push

# CI builda automaticamente. Em ~3-5min, UNRAID tem :edge nova.
# Você valida na UI do UNRAID.
```

### 6.2 Trabalho em feature mais longa

```bash
# Feature branch efêmera — vive enquanto a feature está aberta
git checkout -b feat/006-s3-storage
# … vários commits …
git push -u origin feat/006-s3-storage

# Abrir PR para main. Ao merge, branch é deletado.
# Daí em diante, fluxo 6.1.
```

### 6.3 Promover dev → prod

Quando estiver satisfeito com o que rodou no UNRAID:

```bash
# Verificar exatamente qual sha está rodando em dev
docker exec pkd /pkd -version    # ou inspect da imagem :edge

# Criar tag Git semver (no commit que está rodando em dev)
git tag -a v1.2.3 -m "Release 1.2.3 — descreve mudanças resumidas"
git push origin v1.2.3

# Workflow promote-to-prod (a ser criado) dispara automaticamente:
#   - Pega imagem :sha-abc1234 correspondente
#   - Re-tagueia como :v1.2.3 e :stable
#   - Push para GHCR
# Sem rebuild — re-tag de imagem já existente.

# Em seguida, na EC2:
docker pull ghcr.io/edalcin/pkd:stable
docker stop pkd && docker rm pkd
docker run … (mesmos parâmetros, agora puxando :stable atualizada)
```

### 6.4 Reverter prod (incidente em produção)

```bash
# Identificar última versão estável anterior
docker images ghcr.io/edalcin/pkd

# Apontar :stable para a versão anterior
# (via workflow ou manualmente)

# EC2:
docker pull ghcr.io/edalcin/pkd:v1.2.2
docker stop pkd && docker rm pkd
docker run … --image ghcr.io/edalcin/pkd:v1.2.2
```

Tempo total de reversão: **menos de 1 minuto**, sem mexer em código, sem rebuild.

---

## 7. Mudanças necessárias no projeto (referência futura — **não executar agora**)

Esta seção descreve o que mudaria *quando* a estratégia for adotada. É lista para consulta, não plano de execução imediato.

### 7.1 Em `.github/workflows/build-and-publish.yml`

Adicionar tags `edge` e manter `sha-<curto>`. Decidir se `latest` continua como alias de `edge` (compatibilidade) ou se é removida (mais limpo, mas quebra puxadores externos).

```yaml
tags: |
  type=raw,value=edge
  type=sha,prefix=sha-,format=short
  # type=raw,value=latest    # opcional, para compatibilidade
```

### 7.2 Novo workflow `.github/workflows/promote-to-prod.yml`

Disparado por **tag Git** semver (`v*.*.*`). Faz:

1. Resolve qual `:sha-<curto>` corresponde ao commit da tag.
2. Re-tagueia (`docker buildx imagetools create`) como `:v1.2.3` e `:stable`.
3. Não roda build novo — só re-tag.
4. (Opcional) Cria GitHub Release com changelog.

Esquema mínimo:

```yaml
on:
  push:
    tags: ['v*.*.*']

jobs:
  promote:
    runs-on: ubuntu-latest
    steps:
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Re-tag image
        run: |
          SHORT_SHA=$(echo ${{ github.sha }} | cut -c1-7)
          SRC=ghcr.io/${{ github.repository_owner }}/pkd:sha-$SHORT_SHA
          VER=${{ github.ref_name }}
          docker buildx imagetools create \
            --tag ghcr.io/${{ github.repository_owner }}/pkd:$VER \
            --tag ghcr.io/${{ github.repository_owner }}/pkd:stable \
            $SRC
```

### 7.3 Configuração do container UNRAID

Em `docs/unraid-install.md` (a ser atualizado posteriormente), trocar:

```
Repository: ghcr.io/edalcin/pkd:latest
```

por:

```
Repository: ghcr.io/edalcin/pkd:edge
```

E configurar **Watchtower** ou pull periódico para reflectir novos pushes em `main` automaticamente.

### 7.4 Configuração do container EC2

Apontar para `:stable` (ou explicitamente para `:v1.2.3`):

```
ghcr.io/edalcin/pkd:stable
```

EC2 **não** deve ter atualização automática — pull é evento manual após release tag.

### 7.5 `Dockerfile`

**Não muda.** Continua o mesmo, multi-stage, distroless, sem CGO. A configuração ambiental já cobre as diferenças entre os ambientes.

### 7.6 `CLAUDE.md`

**Continua válida.** A regra "apenas main de longa duração" permanece. Adicionar (quando adotar) uma frase esclarecedora: *"feature branches efêmeras são permitidas durante o desenvolvimento de mudanças maiores, desde que sejam fechadas via PR e deletadas em até alguns dias"*.

---

## 8. Plano B — quando branchear de verdade faz sentido

Existem cenários — raros — onde dois branches Git de longa duração são justificáveis:

### Cenário B1 — hotfix urgente em prod com main já avançado

`main` tem 30 commits novos com refatoração grande, ainda sem promover. Surge bug crítico em prod. Não dá para promover `main` (vai quebrar mais coisa); precisa de fix isolado.

**Solução enxuta:** branch `hotfix/v1.2.4` a partir da tag Git `v1.2.3`, fix mínimo, tag `v1.2.4`, promote-to-prod, deletar branch. Cherry-pick do fix de volta para `main`. **Branch viveu horas, não viola "só main long-lived".**

### Cenário B2 — experimento longo com risco real para prod

Quer experimentar mudança grande (ex: trocar SQLite por outro banco) sem risco de vazar para prod por engano. Aqui um branch `experimental/<x>` com vida longa pode ser justificado, mas:

1. É **branch de experimento**, não de ambiente.
2. Não tem deploy automático — não está conectado a nenhum dos ambientes.
3. Decide-se merge ou descarte ao fim do experimento.

### Quando **NÃO** brancar mesmo se tentado

- "Quero testar uma config diferente em dev." → use env var, não branch.
- "Quero ativar feature em dev mas não em prod." → use feature flag em runtime, não branch.
- "Quero que dev tenha logs verbosos." → use `LOG_LEVEL`, não branch.
- "Quero ter uma versão dev e uma prod." → tags Docker resolvem; branch só piora.

A pergunta sempre é: **"o código-fonte precisa ser literalmente diferente entre os dois ambientes?"** Se a resposta é não, não use branch.

---

## 9. Alternativas avaliadas

| Estratégia | Como funciona | Por que **não** para o PKD |
|---|---|---|
| **GitHub Flow puro** | Só `main`, deploy a cada merge. | É o que existe hoje. Não cria janela de validação entre dev e prod. |
| **GitFlow** | `develop`, `main`, `release/*`, `hotfix/*`, `feature/*`. | Pesado demais para solo dev; otimizado para times grandes com release windows formais. |
| **Branch-per-environment** (`develop` → UNRAID, `main` → EC2) | Dois branches long-lived, merge `develop` → `main` para promover. | Exige sincronização constante entre dois branches; alto overhead de merge. Faz sentido só com divergência real de código. |
| **Trunk + tags promovidas** ✅ | `main` único + tags Docker `:edge`/`:stable`. Promoção = re-tag. | **Recomendado.** Resolve o problema (janela de validação) sem custo de gerenciar dois branches. |
| **Dockerfiles separados (dev/prod)** | `Dockerfile.dev` e `Dockerfile.prod` distintos. | Multiplicaria mantenedoria sem ganho — código é idêntico, só config muda. Anti-pattern para apps ambient-driven. |

---

## 10. Boas práticas de commit e versionamento

### 10.1 Commits em `main` (mantém prática atual)

- Pequenos, atômicos, mensagem em formato Conventional Commits (`feat:`, `fix:`, `docs:` etc.).
- Tudo entra em `main` direto (mudanças pequenas) ou via PR de feature branch (mudanças maiores).

### 10.2 Tags Git semver (nova convenção, ao adotar)

- **Major** (`v2.0.0`): mudança que quebra dado, schema, ou API pública.
- **Minor** (`v1.3.0`): nova feature backward-compatible.
- **Patch** (`v1.2.4`): bugfix.
- A tag Git é criada **só** quando se decide promover para prod. Não há tag por commit.

### 10.3 Anotação de release

Cada tag Git deve ter mensagem (`git tag -a`) com 3-10 linhas resumindo o que vai para prod. Vira changelog automático.

```bash
git tag -a v1.3.0 -m "Release 1.3.0

- feat: armazenamento de anexos em S3 (backend opcional)
- fix: corrige cabeçalho Range em PDFs servidos via S3
- chore: atualiza aws-sdk-go-v2 para v1.x.y
"
```

---

## 11. Riscos e mitigação

| Risco | Severidade | Mitigação |
|---|:---:|---|
| Esquecer de promover; prod fica meses atrás | Média | Lembrete semanal; dashboard com `git log :stable..main --oneline` |
| Promover sem testar; bug chega em prod | Alta | Checklist de validação no UNRAID antes de criar tag Git; release tag implica "validei" |
| `:edge` tornar-se sinônimo de `:latest` na prática (EC2 puxando errado) | Alta | Configurar EC2 explicitamente com `:stable` ou tag semver; **nunca** com `:latest` ou `:edge` |
| Imagem `:sha-<curto>` ser apagada (GHCR retention policy) antes de promover | Média | Configurar retention de tags `sha-*` para >30 dias; ou promover dentro de 7 dias |
| Acabar promovendo um commit errado | Baixa | Tag Git inclui sha implícito; `git tag -a` no commit certo (não em `HEAD` cego) |
| Confusão entre tag Git e tag Docker | Baixa | Mesma string em ambas (`v1.2.3`); workflow `promote-to-prod` automatiza para evitar erro humano |

---

## 12. Glossário

- **Branch (Git):** linha paralela de desenvolvimento dentro de um repositório.
- **Branch long-lived:** branch que existe por meses/anos (ex: `main`, `develop`).
- **Branch efêmero:** branch criado para uma tarefa específica, deletado ao concluir (geralmente em horas/dias).
- **Tag (Git):** rótulo imutável apontando para um commit específico (ex: `v1.2.3`).
- **Tag (Docker):** rótulo apontando para uma imagem específica em um registry. Pode ser **móvel** (`:latest`, `:edge`, `:stable`) ou **imutável** por convenção (`:sha-abc1234`, `:v1.2.3`).
- **Release:** marco formal de versão; tipicamente uma tag Git semver + uma tag Docker semver.
- **Promoção:** ato de declarar uma versão pronta para um ambiente "mais sério". No PKD, dev → prod.
- **Re-tag:** apontar nova tag para imagem que já existe (sem rebuild).
- **Trunk:** o branch principal de longa duração (no PKD, `main`).
- **Trunk-based development:** estilo onde todo trabalho converge para um único branch principal, com gates de qualidade (CI, testes, code review) protegendo a entrada.
- **Feature flag:** chave em runtime que liga/desliga código sem recompilar — alternativa a divergir código entre branches.
- **Environment / ambiente:** instância em execução do software (no PKD, "dev no UNRAID" e "prod na EC2").
- **GHCR:** GitHub Container Registry, onde o PKD publica imagens (`ghcr.io/edalcin/pkd`).

---

## 13. Checklist de adoção (referência futura — **não executar agora**)

Quando decidir adotar a estratégia, executar **na ordem**:

1. [ ] Atualizar `.github/workflows/build-and-publish.yml` para publicar `:edge` (e manter `:sha-*` imutável).
2. [ ] Criar `.github/workflows/promote-to-prod.yml` disparado por tag `v*.*.*`.
3. [ ] Configurar GHCR retention: `sha-*` → 30 dias mínimo; `v*` → indefinido; `edge`/`stable`/`latest` → indefinido.
4. [ ] Atualizar `docs/unraid-install.md` para usar `:edge`.
5. [ ] Atualizar configuração do container UNRAID (campo Repository) para `:edge`.
6. [ ] Documentar em `docs/operations.md` o procedimento de promoção (passo-a-passo de criar tag Git + verificar prod).
7. [ ] Configurar EC2 para `:stable` (campo de imagem do container).
8. [ ] Criar primeira tag Git semver (`v1.0.0` ou versão atual congelada) e promover.
9. [ ] Atualizar `CLAUDE.md` com a nota sobre feature branches efêmeras serem permitidas.
10. [ ] (Opcional) Criar dashboard simples mostrando `commits between :stable and :edge`.

---

## 14. Referências cruzadas

- [`docs/s3-attachments-feasibility.md`](./s3-attachments-feasibility.md) — motivador principal para precisar de janela de validação dev→prod.
- [`docs/operations.md`](./operations.md) — procedimentos operacionais do dia-a-dia (quando atualizar, ganhará seção sobre promoção).
- [`docs/security.md`](./security.md) — postura de segurança; tag-based deployment reduz blast radius.
- [`docs/unraid-install.md`](./unraid-install.md) — configuração da instância de dev.
- [`CLAUDE.md`](../CLAUDE.md) — convenções de desenvolvimento; este documento complementa, não contradiz.

---

## 15. Conclusão

A introdução de S3 + dois ambientes operacionais não exige quebrar a regra "apenas `main` long-lived". Exige apenas reconhecer que **o que precisa ser separado é o release, não o código**. Tags Docker resolvem isso elegantemente: o mesmo binário roda nos dois ambientes, mas dev recebe imediatamente e prod recebe só quando você decidir.

Branches Git permanecem como ferramenta para isolar trabalho em curso (efêmeros) ou para casos extraordinários como hotfix urgente — não como mecanismo padrão de separação de ambientes.

Quando este documento for revisitado para implementação, o checklist em §13 está pronto para servir de roteiro.
