<script>
  import MemoryDateFields from './MemoryDateFields.svelte'
  import { createMemory, buildMemoryDate } from '../stores/memories.js'
  import { ApiError } from '../api.js'

  let { onClose, onCreated } = $props()

  const now = new Date()
  let year = $state(now.getFullYear())
  let month = $state(now.getMonth() + 1)
  let day = $state(null)
  let momento = $state('nenhum')
  let time = $state('')
  let period = $state('')
  let title = $state('')
  let saving = $state(false)
  let error = $state('')

  async function handleSubmit(e) {
    e.preventDefault()
    if (saving) return
    const trimmed = title.trim()
    if (!trimmed || !year) return
    saving = true
    error = ''
    try {
      const date = buildMemoryDate({ year, month, day, momento, time, period })
      const doc = await createMemory(trimmed, date)
      onCreated?.(doc)
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Erro ao criar memória.'
    } finally {
      saving = false
    }
  }
</script>

<div class="modal-backdrop" onclick={onClose} role="dialog" aria-modal="true" aria-label="Nova Memória">
  <div class="modal" onclick={e => e.stopPropagation()}>
    <h2>🗓️ Nova Memória</h2>

    <form onsubmit={handleSubmit}>
      <MemoryDateFields
        bind:year
        bind:month
        bind:day
        bind:momento
        bind:time
        bind:period
        disabled={saving}
        focusDay={true}
      />

      <label class="memdate-field" style="margin-bottom:.75rem">
        <span class="memdate-label">Título *</span>
        <input
          class="memdate-input"
          type="text"
          bind:value={title}
          placeholder="Título da memória"
          aria-label="Título"
          disabled={saving}
          required
        />
      </label>

      {#if error}
        <p class="new-memory-error">{error}</p>
      {/if}

      <div class="modal-actions">
        <button type="button" class="btn btn-ghost" onclick={onClose} disabled={saving}>Cancelar</button>
        <button type="submit" class="btn btn-primary" disabled={saving || !title.trim() || !year}>
          {saving ? 'Criando…' : 'Criar'}
        </button>
      </div>
    </form>
  </div>
</div>

<style>
  .memdate-field {
    display: flex;
    flex-direction: column;
    gap: .25rem;
  }

  .memdate-label {
    font-size: .75rem;
    color: var(--text-muted);
  }

  .memdate-input {
    padding: .4rem .5rem;
    font-size: .875rem;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    width: 100%;
  }

  .new-memory-error {
    color: var(--danger);
    font-size: .8rem;
    margin: -.25rem 0 .75rem;
  }
</style>
