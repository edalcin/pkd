<script>
  let { conflict, onNavigate, onClose } = $props()
</script>

<div class="modal-backdrop" onclick={onClose} role="dialog" aria-modal="true" aria-label="Título duplicado">
  <div class="modal" onclick={e => e.stopPropagation()}>
    <h2>Título duplicado</h2>
    <p>
      Já existe um documento com o título
      <strong>"{conflict.existing_doc.title}"</strong>.
    </p>
    <div class="modal-actions">
      <button class="btn btn-ghost" onclick={onClose}>Cancelar</button>
      <button class="btn btn-primary" onclick={onNavigate}>Ver documento existente</button>
    </div>
  </div>
</div>
