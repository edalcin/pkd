# Share Recursive Flag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adicionar flag `include_children` ao link de compartilhamento público, dando ao usuário controle sobre se sub-documentos são incluídos (com acesso público e visíveis na página pública) ou não.

**Architecture:** A flag é persistida na coluna `include_children` da tabela `share_links` (migration idempotente). O handler de criação lê o campo do body JSON; o handler da página pública usa a flag para omitir filhos. O frontend adiciona um checkbox no `ShareDialog` com default `true`.

**Tech Stack:** Go (backend), Svelte (frontend), SQLite (store), httptest (integration tests).

## Global Constraints

- Nunca criar novo branch — commitar sempre no main.
- Zero breaking changes: links existentes mantêm `include_children = 1` via `DEFAULT`.
- Seguir padrão de migration do projeto: adicionar entrada ao slice `colMigrations` em `migrate.go` (ignora "duplicate column name").
- Não criar mocks nos testes — usar servidor em memória como os demais integration tests.
- Não rodar `go test ./...` — rodar apenas os testes adicionados/modificados.

---

## Arquivos Alterados

| Arquivo | Ação |
|---|---|
| `internal/store/migrate.go` | Adiciona migration `include_children` ao slice `colMigrations` |
| `internal/model/share.go` | Adiciona campo `IncludeChildren bool` ao `ShareLink` |
| `internal/store/shares.go` | `Create()` aceita `includeChildren bool`; `LookupByToken()` inclui coluna no SELECT/Scan |
| `internal/server/handlers_share.go` | `handleCreateShare()` lê body; `handlePublicShare()` respeita a flag |
| `frontend/src/lib/components/ShareDialog.svelte` | Checkbox + badge pós-criação |
| `tests/integration/share_test.go` | Novo arquivo com testes de integração |

---

### Task 1: Migration e Model

**Files:**
- Modify: `internal/store/migrate.go:85-109`
- Modify: `internal/model/share.go:1-31`

**Interfaces:**
- Produces: `model.ShareLink.IncludeChildren bool` — usado pelas Tasks 2 e 3.

---

- [ ] **Step 1: Adicionar migration em `migrate.go`**

No slice `colMigrations` (linhas 85–101), adicionar entrada após a última:

```go
{`ALTER TABLE share_links ADD COLUMN include_children INTEGER NOT NULL DEFAULT 1`, "alter share_links include_children"},
```

O bloco completo fica:
```go
colMigrations := []struct {
    sql string
    ctx string
}{
    {`ALTER TABLE documents ADD COLUMN is_favorite INTEGER NOT NULL DEFAULT 0`, "alter documents is_favorite"},
    {`ALTER TABLE document_links ADD COLUMN manual INTEGER NOT NULL DEFAULT 0`, "alter document_links"},
    {`ALTER TABLE tags ADD COLUMN color TEXT NOT NULL DEFAULT ''`, "alter tags color"},
    {`ALTER TABLE share_links ADD COLUMN token_plain TEXT NOT NULL DEFAULT ''`, "alter share_links token_plain"},
    {`ALTER TABLE share_links ADD COLUMN is_auto INTEGER NOT NULL DEFAULT 0`, "alter share_links is_auto"},
    {`ALTER TABLE documents ADD COLUMN assoc_year  INTEGER`, "alter documents assoc_year"},
    {`ALTER TABLE documents ADD COLUMN assoc_month INTEGER`, "alter documents assoc_month"},
    {`ALTER TABLE documents ADD COLUMN assoc_day   INTEGER`, "alter documents assoc_day"},
    {`ALTER TABLE documents ADD COLUMN locked      INTEGER NOT NULL DEFAULT 0`, "alter documents locked"},
    {`ALTER TABLE documents ADD COLUMN archived_at TEXT`, "alter documents archived_at"},
    {`ALTER TABLE attachments ADD COLUMN storage_location TEXT NOT NULL DEFAULT 'local'`, "alter attachments storage_location"},
    {`ALTER TABLE attachments ADD COLUMN content_sha256 TEXT`, "alter attachments content_sha256"},
    {`ALTER TABLE share_links ADD COLUMN include_children INTEGER NOT NULL DEFAULT 1`, "alter share_links include_children"},
}
```

- [ ] **Step 2: Adicionar campo `IncludeChildren` ao model**

Substituir o conteúdo de `internal/model/share.go`:

```go
package model

import "time"

// ShareLink mirrors the share_links table.
type ShareLink struct {
	ID              int64      `json:"id"`
	DocumentID      int64      `json:"document_id"`
	IncludeChildren bool       `json:"include_children"`
	TokenHash       []byte     `json:"-"` // never sent to clients
	CreatedAt       time.Time  `json:"created_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}

// ShareCreateResponse is returned once when a share link is created.
// The plaintext token is shown exactly once and never stored.
type ShareCreateResponse struct {
	Token    string `json:"token"`
	URL      string `json:"url"`
	RevokeID int64  `json:"revoke_id"`
}

// ShareWithDoc is returned by the admin list-shares endpoint.
// It enriches a ShareLink with the associated document title and the full public URL.
type ShareWithDoc struct {
	ID            int64     `json:"id"`
	DocumentID    int64     `json:"document_id"`
	DocumentTitle string    `json:"document_title"`
	CreatedAt     time.Time `json:"created_at"`
	Token         string    `json:"token"`         // plaintext token; empty for shares created before this field was added
	URL           string    `json:"url,omitempty"` // full public URL, populated by the handler
}
```

- [ ] **Step 3: Verificar compilação**

```bash
cd D:/git/pkd && go build ./...
```

Esperado: sem erros.

- [ ] **Step 4: Commit**

```bash
cd D:/git/pkd && git add internal/store/migrate.go internal/model/share.go
git commit -m "feat(share): add include_children column and model field"
```

---

### Task 2: Store — `Create()` e `LookupByToken()`

**Files:**
- Modify: `internal/store/shares.go`

**Interfaces:**
- Consumes: `model.ShareLink.IncludeChildren bool` (Task 1)
- Produces: `ShareStore.Create(docID int64, includeChildren bool)` — consumido pela Task 3.
- Produces: `LookupByToken()` retorna `ShareLink` com `IncludeChildren` preenchido — consumido pela Task 3.

---

- [ ] **Step 1: Atualizar `Create()` para aceitar e persistir `includeChildren`**

Substituir a função `Create` (linhas 26–53 do arquivo atual):

```go
// Create generates a share link for docID. Returns the plaintext token (shown
// once) and the ShareLink record containing the row ID needed for revocation.
// The token is NOT stored — only its SHA-256 hash is persisted.
// includeChildren controls whether sub-document auto-shares are created.
func (s *ShareStore) Create(docID int64, includeChildren bool) (plaintext string, share *model.ShareLink, err error) {
	plaintext = security.NewToken(32) // 32 bytes → 43 chars base64url
	hash := security.HashSHA256(plaintext)

	ic := 0
	if includeChildren {
		ic = 1
	}

	now := time.Now().UTC()
	var id int64
	err = WithTx(s.db, func(tx *sql.Tx) error {
		res, err := tx.Exec(`
			INSERT INTO share_links (document_id, token_hash, token_plain, include_children, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			docID, hash, plaintext, ic, now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	share = &model.ShareLink{
		ID:              id,
		DocumentID:      docID,
		IncludeChildren: includeChildren,
		TokenHash:       hash,
		CreatedAt:       now,
	}
	return plaintext, share, nil
}
```

- [ ] **Step 2: Atualizar `LookupByToken()` para incluir `include_children` no SELECT**

Substituir a função `LookupByToken` (linhas 57–86 do arquivo atual):

```go
// LookupByToken finds an active (non-revoked) share link by plaintext token.
// Returns ErrNotFound if the token does not match any active link.
func (s *ShareStore) LookupByToken(plaintext string) (*model.ShareLink, error) {
	hash := security.HashSHA256(plaintext)

	rows, err := s.db.Query(`
		SELECT id, document_id, token_hash, include_children, created_at
		FROM share_links
		WHERE revoked_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sl model.ShareLink
		var tokenHash []byte
		var createdStr string
		var ic int
		if err := rows.Scan(&sl.ID, &sl.DocumentID, &tokenHash, &ic, &createdStr); err != nil {
			return nil, err
		}
		if subtle.ConstantTimeCompare(hash, tokenHash) == 1 {
			sl.TokenHash = tokenHash
			sl.IncludeChildren = ic == 1
			sl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
			return &sl, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, ErrNotFound
}
```

- [ ] **Step 3: Verificar compilação**

```bash
cd D:/git/pkd && go build ./...
```

Esperado: sem erros de compilação. Pode haver erro de compilação em `handlers_share.go` se ele chama `shares.Create(docID)` — será corrigido na Task 3.

- [ ] **Step 4: Commit**

```bash
cd D:/git/pkd && git add internal/store/shares.go
git commit -m "feat(share): update Create/LookupByToken to handle include_children"
```

---

### Task 3: Handlers — criação e página pública

**Files:**
- Modify: `internal/server/handlers_share.go`

**Interfaces:**
- Consumes: `ShareStore.Create(docID int64, includeChildren bool)` (Task 2)
- Consumes: `model.ShareLink.IncludeChildren bool` (Task 1)

---

- [ ] **Step 1: Atualizar `handleCreateShare()` para ler `include_children` do body**

Substituir a função `handleCreateShare` (linhas 59–91 do arquivo atual):

```go
func (s *Server) handleCreateShare() http.HandlerFunc {
	type createShareRequest struct {
		IncludeChildren *bool `json:"include_children"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if _, err := s.docs.GetByID(docID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req createShareRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}
		// Default: include children (backward-compatible).
		includeChildren := true
		if req.IncludeChildren != nil {
			includeChildren = *req.IncludeChildren
		}

		plaintext, share, err := s.shares.Create(docID, includeChildren)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Auto-create shares for all descendants only when recursive.
		if includeChildren {
			for _, id := range s.collectDescendantIDs(docID) {
				_ = s.shares.CreateAuto(id)
			}
		}

		base := s.baseURL(r)
		shareURL := base + "public/" + plaintext

		writeJSON(w, http.StatusCreated, model.ShareCreateResponse{
			Token:    plaintext,
			URL:      shareURL,
			RevokeID: share.ID,
		})
	}
}
```

Adicionar `"encoding/json"` ao bloco de imports se ainda não estiver presente (verificar o import atual do arquivo).

- [ ] **Step 2: Atualizar `handlePublicShare()` para respeitar `include_children`**

Na função `handlePublicShare`, após a linha que busca o `doc` (aproximadamente linha 175), inserir a lógica de short-circuit para filhos:

Localizar o bloco que começa com `// Fetch children and ensure each has an active public share.` (linhas ~174–196) e substituí-lo por:

```go
// Fetch children only if the share was created with include_children=true.
var childData []shareChildData
if shareLink.IncludeChildren {
	children, err := s.docs.ListChildren(doc.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, child := range children {
		childToken, cerr := s.shares.GetActiveShareForDocument(child.ID)
		if cerr != nil || childToken == "" {
			continue // only list children that already have an explicit share
		}
		childIcon := child.Icon
		if childIcon == "" {
			childIcon = "📄"
		}
		childData = append(childData, shareChildData{
			Title:         child.Title,
			Icon:          childIcon,
			IconIsBoxicon: isBoxicon(childIcon),
			URL:           base + "public/" + childToken,
		})
	}
}
```

A variável `childData` não é mais declarada antes deste bloco — remover a declaração `var childData []shareChildData` original se existir separada.

- [ ] **Step 3: Verificar compilação**

```bash
cd D:/git/pkd && go build ./...
```

Esperado: sem erros.

- [ ] **Step 4: Commit**

```bash
cd D:/git/pkd && git add internal/server/handlers_share.go
git commit -m "feat(share): handleCreateShare reads include_children; public page respects flag"
```

---

### Task 4: Testes de integração

**Files:**
- Create: `tests/integration/share_test.go`

**Interfaces:**
- Consumes: servidor de teste `ts` (definido em `auth_test.go`), helpers `loginClient`, `apiPost`, `apiDelete`, `itoa` (definidos em `documents_crud_test.go`).

---

- [ ] **Step 1: Escrever os testes**

Criar `tests/integration/share_test.go`:

```go
package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestShareRecursive verifica que criar um share com include_children=true
// gera auto-shares para os descendentes, e a página pública lista os filhos.
func TestShareRecursive(t *testing.T) {
	client := loginClient(t)

	// Cria documento pai
	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Share Parent",
	}))
	parentID := int64(parent["id"].(float64))

	// Cria filho
	child := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID,
		"title":     "Share Child",
	}))
	childID := int64(child["id"].(float64))
	_ = childID

	// Gera share recursivo (default)
	shareResp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/shares", map[string]interface{}{
		"include_children": true,
	})
	if shareResp.StatusCode != http.StatusCreated {
		t.Fatalf("create share: want 201, got %d", shareResp.StatusCode)
	}
	var shareData map[string]interface{}
	json.NewDecoder(shareResp.Body).Decode(&shareData)
	shareResp.Body.Close()

	shareURL, _ := shareData["url"].(string)
	if shareURL == "" {
		t.Fatal("share URL should not be empty")
	}

	// Acessa a página pública
	pubResp, err := http.Get(shareURL)
	if err != nil {
		t.Fatalf("public page GET: %v", err)
	}
	defer pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("public page: want 200, got %d", pubResp.StatusCode)
	}

	// A página deve conter o título do filho (share automático foi criado)
	var buf strings.Builder
	buf.ReadFrom(pubResp.Body)
	if !strings.Contains(buf.String(), "Share Child") {
		t.Error("public page should list child document when include_children=true")
	}
}

// TestShareNonRecursive verifica que criar um share com include_children=false
// NÃO gera auto-shares para os descendentes, e a página pública não lista filhos.
func TestShareNonRecursive(t *testing.T) {
	client := loginClient(t)

	// Cria documento pai
	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "NonRec Parent",
	}))
	parentID := int64(parent["id"].(float64))

	// Cria filho
	_ = decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID,
		"title":     "NonRec Child",
	}))

	// Gera share NÃO recursivo
	shareResp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/shares", map[string]interface{}{
		"include_children": false,
	})
	if shareResp.StatusCode != http.StatusCreated {
		t.Fatalf("create non-recursive share: want 201, got %d", shareResp.StatusCode)
	}
	var shareData map[string]interface{}
	json.NewDecoder(shareResp.Body).Decode(&shareData)
	shareResp.Body.Close()

	shareURL, _ := shareData["url"].(string)
	if shareURL == "" {
		t.Fatal("share URL should not be empty")
	}

	// Acessa a página pública
	pubResp, err := http.Get(shareURL)
	if err != nil {
		t.Fatalf("public page GET: %v", err)
	}
	defer pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("public page: want 200, got %d", pubResp.StatusCode)
	}

	// A página NÃO deve conter o título do filho
	var buf strings.Builder
	buf.ReadFrom(pubResp.Body)
	if strings.Contains(buf.String(), "NonRec Child") {
		t.Error("public page should NOT list child document when include_children=false")
	}
}

// TestShareDefaultIsRecursive verifica que omitir include_children no body
// resulta em comportamento recursivo (backward-compatible).
func TestShareDefaultIsRecursive(t *testing.T) {
	client := loginClient(t)

	parent := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Default Parent",
	}))
	parentID := int64(parent["id"].(float64))

	_ = decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": parentID,
		"title":     "Default Child",
	}))

	// POST sem body (simula cliente antigo)
	shareResp := apiPost(t, client, "/api/documents/"+itoa(parentID)+"/shares", nil)
	if shareResp.StatusCode != http.StatusCreated {
		t.Fatalf("create default share: want 201, got %d", shareResp.StatusCode)
	}
	var shareData map[string]interface{}
	json.NewDecoder(shareResp.Body).Decode(&shareData)
	shareResp.Body.Close()

	shareURL, _ := shareData["url"].(string)

	pubResp, _ := http.Get(shareURL)
	defer pubResp.Body.Close()

	var buf strings.Builder
	buf.ReadFrom(pubResp.Body)
	if !strings.Contains(buf.String(), "Default Child") {
		t.Error("public page should list child when no body sent (default=recursive)")
	}
}
```

- [ ] **Step 2: Rodar os testes para verificar que passam**

```bash
cd D:/git/pkd && go test ./tests/integration/ -run "TestShare" -v
```

Esperado:
```
--- PASS: TestShareRecursive (...)
--- PASS: TestShareNonRecursive (...)
--- PASS: TestShareDefaultIsRecursive (...)
PASS
```

- [ ] **Step 3: Commit**

```bash
cd D:/git/pkd && git add tests/integration/share_test.go
git commit -m "test(share): integration tests for include_children flag"
```

---

### Task 5: Frontend — ShareDialog.svelte

**Files:**
- Modify: `frontend/src/lib/components/ShareDialog.svelte`

**Interfaces:**
- Produces: POST `/api/documents/:id/shares` com body `{ include_children: boolean }`.

---

- [ ] **Step 1: Atualizar o componente**

Substituir o conteúdo completo de `frontend/src/lib/components/ShareDialog.svelte`:

```svelte
<script>
  import { apiPost, apiDelete } from '../api.js'

  let { docId, onClose } = $props()

  let shareUrl = $state('')
  let shareId = $state(null)
  let loading = $state(false)
  let copied = $state(false)
  let includeChildren = $state(true)
  let wasRecursive = $state(false)

  async function generateLink() {
    loading = true
    try {
      const data = await apiPost(`/api/documents/${docId}/shares`, {
        include_children: includeChildren,
      })
      shareUrl = data.url || `${window.location.origin}/public/${data.token}`
      shareId = data.revoke_id
      wasRecursive = includeChildren
    } finally {
      loading = false
    }
  }

  async function revokeLink() {
    if (!shareId || !confirm('Revogar o link de compartilhamento?')) return
    await apiDelete(`/api/documents/${docId}/shares/${shareId}`)
    shareUrl = ''
    shareId = null
  }

  async function copyLink() {
    await navigator.clipboard.writeText(shareUrl)
    copied = true
    setTimeout(() => { copied = false }, 2000)
  }
</script>

<div class="modal-backdrop" onclick={onClose} role="dialog" aria-modal="true" aria-label="Compartilhar documento">
  <div class="modal" onclick={e => e.stopPropagation()}>
    <h2>🔗 Compartilhar documento</h2>
    <p class="share-info">
      Gere um link público de leitura para este documento.<br>
      Qualquer pessoa com o link pode visualizar o conteúdo.
    </p>

    {#if shareUrl}
      <div class="share-scope-badge">
        {#if wasRecursive}
          🔁 Inclui sub-documentos
        {:else}
          📄 Somente este documento
        {/if}
      </div>
      <div class="share-url-row">
        <input class="share-url-input" type="text" readonly value={shareUrl} />
        <button class="btn btn-primary" onclick={copyLink}>
          {copied ? '✓ Copiado!' : 'Copiar'}
        </button>
      </div>
      <div class="modal-actions">
        <button class="btn btn-danger" onclick={revokeLink}>Revogar link</button>
        <button class="btn btn-ghost" onclick={onClose}>Fechar</button>
      </div>
    {:else}
      <label class="share-children-label">
        <input type="checkbox" bind:checked={includeChildren} />
        Incluir sub-documentos (recursivo)
      </label>
      <p class="share-children-hint">
        {#if includeChildren}
          Os filhos também ficam acessíveis publicamente e aparecem listados neste documento.
        {:else}
          Somente este documento será acessível. Os filhos não aparecerão no link.
        {/if}
      </p>
      <div class="modal-actions">
        <button class="btn btn-ghost" onclick={onClose}>Cancelar</button>
        <button class="btn btn-primary" onclick={generateLink} disabled={loading}>
          {loading ? 'Gerando…' : 'Gerar link'}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  .share-info {
    font-size: .875rem;
    color: var(--text-muted);
    margin-bottom: 1rem;
    line-height: 1.5;
  }

  .share-children-label {
    display: flex;
    align-items: center;
    gap: .5rem;
    font-size: .875rem;
    cursor: pointer;
    margin-bottom: .375rem;
  }

  .share-children-hint {
    font-size: .8rem;
    color: var(--text-muted);
    margin-bottom: 1rem;
    padding-left: 1.5rem;
    line-height: 1.4;
  }

  .share-scope-badge {
    display: inline-block;
    font-size: .75rem;
    color: var(--text-muted);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: .2rem .6rem;
    margin-bottom: .75rem;
  }

  .share-url-row {
    display: flex;
    gap: .5rem;
    margin-bottom: .75rem;
  }

  .share-url-input {
    flex: 1;
    padding: .45rem .75rem;
    font-size: .875rem;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
  }
</style>
```

- [ ] **Step 2: Verificar build do frontend**

```bash
cd D:/git/pkd/frontend && npm run build
```

Esperado: build sem erros.

- [ ] **Step 3: Commit**

```bash
cd D:/git/pkd && git add frontend/src/lib/components/ShareDialog.svelte
git commit -m "feat(share): add include_children checkbox and scope badge to ShareDialog"
```

---

## Self-Review

### Spec coverage
- [x] Flag `include_children` na tabela `share_links` — Task 1 migration
- [x] `model.ShareLink.IncludeChildren` — Task 1 model
- [x] `Create()` aceita `includeChildren bool` — Task 2
- [x] `LookupByToken()` inclui coluna — Task 2
- [x] `handleCreateShare()` lê body JSON — Task 3
- [x] `handlePublicShare()` omite filhos quando `false` — Task 3
- [x] Frontend: checkbox (default `true`) + badge pós-criação — Task 5
- [x] Compatibilidade com links existentes (`DEFAULT 1`) — Task 1 migration
- [x] Compatibilidade com body ausente (default `true`) — Task 3
- [x] Testes: recursivo, não-recursivo, default — Task 4

### Consistência de tipos
- `Create(docID int64, includeChildren bool)` — definido Task 2, consumido Task 3 ✓
- `model.ShareLink.IncludeChildren bool` — definido Task 1, lido Task 2 (LookupByToken) e Task 3 (handlePublicShare) ✓
- `ic int` (0/1) ↔ `include_children INTEGER` ↔ `ic == 1` → `bool` — conversão explícita em ambos os sentidos ✓

### Sem placeholders
Todos os passos contêm código completo. ✓
