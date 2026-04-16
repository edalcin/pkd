<script>
  import { apiPost, apiDelete } from '../api.js'

  let { docId, onClose } = $props()

  let shareUrl = $state('')
  let shareId = $state(null)
  let loading = $state(false)
  let copied = $state(false)

  async function generateLink() {
    loading = true
    try {
      const data = await apiPost(`/api/documents/${docId}/shares`)
      shareUrl = data.url || `${window.location.origin}/public/${data.token}`
      shareId = data.revoke_id
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
