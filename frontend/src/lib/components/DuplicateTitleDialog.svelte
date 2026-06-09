<script>
  let { conflict, onNavigate, onUseSuggestion, onClose } = $props()
</script>

<div class="modal-backdrop" onclick={onClose} role="dialog" aria-modal="true" aria-label="Título duplicado">
  <div class="modal" onclick={e => e.stopPropagation()}>
    <h2>Título duplicado</h2>
    <p>
      Já existe um documento com o título
      <strong>"{conflict.existing_doc.title}"</strong>.
    </p>

    {#if conflict.suggestions?.length}
      <div class="suggestions">
        <p class="suggestions-label">Sugestões de título único:</p>
        <div class="suggestion-list">
          {#each conflict.suggestions as suggestion}
            <button class="btn btn-ghost suggestion-btn" onclick={() => onUseSuggestion(suggestion)}>
              {suggestion}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <div class="modal-actions">
      <button class="btn btn-ghost" onclick={onClose}>Cancelar</button>
      <button class="btn btn-primary" onclick={onNavigate}>Ver documento existente</button>
    </div>
  </div>
</div>

<style>
  .suggestions {
    margin-top: 1rem;
    padding-top: .75rem;
    border-top: 1px solid var(--border);
  }
  .suggestions-label {
    font-size: .8rem;
    color: var(--text-muted);
    margin-bottom: .5rem;
  }
  .suggestion-list {
    display: flex;
    flex-wrap: wrap;
    gap: .375rem;
  }
  .suggestion-btn {
    font-size: .8125rem;
    border: 1px solid var(--border);
  }
</style>
