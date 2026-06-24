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