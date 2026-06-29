<script>
  import { apiPost, apiDelete } from '../api.js'

  let { docId, onClose } = $props()

  let shareUrl = $state('')
  let shareId = $state(null)
  let loading = $state(false)
  let copied = $state(false)
  let includeChildren = $state(true)
  let includeParent = $state(false)
  let wasRecursive = $state(false)
  let wasParent = $state(false)

  async function generateLink() {
    const ic = includeChildren
    const ip = includeParent
    loading = true
    try {
      const data = await apiPost(`/api/documents/${docId}/shares`, {
        include_children: ic,
        include_parent:   ip,
      })
      shareUrl = data.url || `${window.location.origin}/public/${data.token}`
      shareId = data.revoke_id
      wasRecursive = ic
      wasParent = ip
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
        {wasRecursive ? '🔁 Inclui sub-documentos' : '📄 Somente este documento'}
        {#if wasParent} · 🔼 Inclui documento-pai{/if}
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
        <input type="checkbox" bind:checked={includeChildren} disabled={loading} />
        Incluir sub-documentos (recursivo)
      </label>
      <p class="share-children-hint">
        {#if includeChildren}
          Os filhos também ficam acessíveis publicamente e aparecem listados neste documento.
        {:else}
          Somente este documento será acessível. Os filhos não aparecerão no link.
        {/if}
      </p>
      <label class="share-children-label">
        <input type="checkbox" bind:checked={includeParent} disabled={loading} />
        Incluir link para documento-pai
      </label>
      <p class="share-children-hint">
        {#if includeParent}
          O link público exibirá um atalho para o documento-pai (se ele também tiver um link ativo).
        {:else}
          O documento-pai não será acessível a partir deste link.
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
