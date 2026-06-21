# Design: Controle de Recursividade no Compartilhamento Público

**Data:** 2026-06-21  
**Status:** Aprovado  

---

## Problema

Ao criar um link de compartilhamento público, o sistema **sempre** gera shares automáticos para todos os sub-documentos descendentes (recursivo via BFS). O usuário não tem controle sobre esse comportamento. A feature adiciona uma opção na hora da criação do link: incluir sub-documentos ou não.

---

## Decisões de Design

| Decisão | Escolha |
|---|---|
| Padrão ao criar | Recursivo (checkbox marcado por padrão) |
| Sub-docs com link próprio em link não-recursivo | Não aparecem na página pública |
| Persistência da intenção | Flag `include_children` na tabela `share_links` |

---

## Arquitetura

### 1. Schema — `share_links`

```sql
ALTER TABLE share_links ADD COLUMN include_children INTEGER NOT NULL DEFAULT 1;
```

`DEFAULT 1` garante que todos os links existentes mantenham o comportamento atual (recursivo). Migração idempotente via verificação de coluna em `migrate.go` (padrão já usado no projeto: `PRAGMA table_info` antes de `ALTER TABLE`).

---

### 2. Model — `internal/model/share.go`

Adicionar campo `IncludeChildren bool` ao `ShareLink`:

```go
type ShareLink struct {
    ID              int64      `json:"id"`
    DocumentID      int64      `json:"document_id"`
    IncludeChildren bool       `json:"include_children"`
    TokenHash       []byte     `json:"-"`
    CreatedAt       time.Time  `json:"created_at"`
    RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}
```

---

### 3. Store — `internal/store/shares.go`

**`Create()` — assinatura atualizada:**
```go
func (s *ShareStore) Create(docID int64, includeChildren bool) (plaintext string, share *model.ShareLink, err error)
```
Persiste `include_children` na linha inserida.

**`LookupByToken()` — query atualizada:**  
Inclui `include_children` no `SELECT` para popular o campo do model.

Demais métodos (`CreateAuto`, `Revoke`, `RevokeAutoForDocument`, `ListAllActive`, `GetActiveShareForDocument`) sem alteração.

---

### 4. Handler de criação — `handleCreateShare()`

**Request body (JSON):**
```json
{ "include_children": true }
```

- Campo ausente ou `null` → default `true` (via `*bool` com nil-check)
- Passa `includeChildren` para `shares.Create()`
- Só chama `collectDescendantIDs` + `CreateAuto` quando `includeChildren = true`

**Revogação (`handleRevokeShare()`):**  
Sem alteração. A cascata de auto-shares só tem efeito quando existem auto-shares (criados somente para links recursivos).

---

### 5. Handler da página pública — `handlePublicShare()`

Após buscar o `shareLink` pelo token:

```go
if !shareLink.IncludeChildren {
    // Omite filhos: não busca children, não renderiza a seção
    childData = nil
}
```

A lógica de resolução do parent link não é afetada — exibir o pai é independente de incluir filhos.

---

### 6. Frontend — `ShareDialog.svelte`

**Estado "sem link" (antes de gerar):**
- Adicionar `let includeChildren = $state(true)` 
- Renderizar checkbox abaixo do texto descritivo:
  ```
  ☑ Incluir sub-documentos (recursivo)
    Quando marcado, os filhos também ficam acessíveis publicamente.
  ```
- Enviar `include_children: includeChildren` no body do `apiPost`

**Estado "com link" (após gerar):**
- Exibir badge informativo: `"🔁 Recursivo"` ou `"📄 Somente este documento"`
- Checkbox oculto (intenção já fixada no link criado)

---

## Fluxo Completo

```
Usuário abre ShareDialog
  └─ Checkbox "Incluir sub-documentos" (marcado por padrão)
     ├─ Marcado → POST /api/documents/:id/shares { include_children: true }
     │   └─ Backend cria share + auto-shares para todos os descendentes
     │   └─ Página pública mostra filhos com links
     └─ Desmarcado → POST /api/documents/:id/shares { include_children: false }
         └─ Backend cria share SOMENTE para o documento raiz
         └─ Página pública não exibe seção de filhos
```

---

## Compatibilidade

- **Links existentes:** `include_children = 1` via `DEFAULT` — comportamento inalterado
- **API:** campo opcional com default `true` — clientes antigos não precisam atualizar
- **Revogação:** cascata de auto-shares funciona corretamente em ambos os casos

---

## Testes

### Existentes (verificar que não quebram)
- Testes de integração em `tests/integration/` que chamam `shares.Create()` — atualizar assinatura

### Novos
1. `Create(docID, false)` → nenhum auto-share criado para descendentes
2. `Create(docID, true)` → auto-shares criados para todos os descendentes (comportamento atual)
3. `handlePublicShare()` com link `include_children=false` → seção de filhos ausente no HTML
4. `handlePublicShare()` com link `include_children=true` → filhos com share ativo listados

---

## Arquivos Alterados

| Arquivo | Mudança |
|---|---|
| `internal/store/migrate.go` | Migration: `ADD COLUMN include_children` |
| `internal/model/share.go` | Campo `IncludeChildren bool` em `ShareLink` |
| `internal/store/shares.go` | `Create()` aceita `includeChildren bool`; `LookupByToken()` lê a coluna |
| `internal/server/handlers_share.go` | `handleCreateShare()` lê body; `handlePublicShare()` respeita a flag |
| `frontend/src/lib/components/ShareDialog.svelte` | Checkbox + badge informativo |
| `tests/integration/` | Atualizar assinatura + novos casos |
