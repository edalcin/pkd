<script>
  import MemoryDateFields from './MemoryDateFields.svelte'
  import { convertNoteToMemory } from '../stores/notes.js'
  import { buildMemoryDate } from '../stores/memories.js'
  import { ApiError } from '../api.js'

  let { noteId, onClose, onConverted } = $props()

  // Q17: fields start empty (not today's date, unlike NewMemoryDialog).
  let year = $state(null)
  let month = $state(null)
  let day = $state(null)
  let momento = $state('nenhum')
  let time = $state('')
  let period = $state('')
  let saving = $state(false)
  let error = $state('')

  async function handleSubmit(e) {
    e.preventDefault()
    if (saving || !year) return
    saving = true
    error = ''
    try {
      const date = buildMemoryDate({ year, month, day, momento, time, period })
      await convertNoteToMemory(noteId, date)
      onConverted?.()
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Erro ao converter nota em memória.'
    } finally {
      saving = false
    }
  }
</script>

<div class="modal-backdrop" onclick={onClose} role="dialog" aria-modal="true" aria-label="Converter Nota em Memória">
  <div class="modal" onclick={e => e.stopPropagation()}>
    <h2>🗓️ Converter em Memória</h2>

    <form onsubmit={handleSubmit}>
      <MemoryDateFields
        bind:year
        bind:month
        bind:day
        bind:momento
        bind:time
        bind:period
        disabled={saving}
      />

      {#if error}
        <p class="new-memory-error">{error}</p>
      {/if}

      <div class="modal-actions">
        <button type="button" class="btn btn-ghost" onclick={onClose} disabled={saving}>Cancelar</button>
        <button type="submit" class="btn btn-primary" disabled={saving || !year}>
          {saving ? 'Convertendo…' : 'Converter'}
        </button>
      </div>
    </form>
  </div>
</div>

<style>
  .new-memory-error {
    color: var(--danger);
    font-size: .8rem;
    margin: -.25rem 0 .75rem;
  }
</style>
