<script>
  // Mural de Notas (Spec #8): every Nota as a card, masonry, in the server's
  // order (favorites first, then created_at desc). #/notas/{id} opens the Nota
  // in a modal with the full Editor. The sidebar's tag/favorite filters drive
  // the list (Sidebar.svelte → loadNotes).
  import { notes, loadNotes } from '../stores/notes.js'
  import { tagFilter, favoriteFilter, loadTree } from '../stores/documents.js'
  import { replaceHash } from '../nav.js'
  import Editor from './Editor.svelte'
  import NewNoteDialog from './NewNoteDialog.svelte'

  let { openId = null } = $props()

  const BLOCK = 40 // cards per infinite-scroll step (client-side only, #4)
  let shown = $state(BLOCK)
  let width = $state(0)
  let sentinel = $state(null)
  let newNoteOpen = $state(false)

  const filtered = $derived($tagFilter.length > 0 || $favoriteFilter)
  const visible = $derived($notes.slice(0, shown))
  // Column count from the central column's width (#6): 1 / 2 / 3 / 4 max.
  const cols = $derived(width < 640 ? 1 : width < 960 ? 2 : width < 1280 ? 3 : 4)

  // A filter change starts again from the first block.
  $effect(() => { $tagFilter; $favoriteFilter; shown = BLOCK })

  $effect(() => {
    if (!sentinel) return
    const io = new IntersectionObserver(entries => {
      if (entries[0].isIntersecting && shown < $notes.length) shown += BLOCK
    }, { rootMargin: '400px' })
    io.observe(sentinel)
    return () => io.disconnect()
  })

  const rtf = new Intl.RelativeTimeFormat('pt-BR', { numeric: 'auto' })
  const UNITS = [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]]
  function relDate(iso) {
    const s = (new Date(iso) - Date.now()) / 1000
    for (const [u, sec] of UNITS) if (Math.abs(s) >= sec) return rtf.format(Math.round(s / sec), u)
    return 'agora'
  }

  function openNote(e, id) {
    if (e.target.closest('a')) return // a Link no corpo opens the link, not the modal
    window.location.hash = `/notas/${id}` // push: browser "back" closes the modal (#5)
  }

  function closeNote() {
    replaceHash('/notas') // replace: the modal is a state of the Mural, not a page (#5)
    loadNotes() // refresh cards after edits in the modal (#7)
  }

  function onKey(e) {
    if (!openId || e.key !== 'Escape') return
    if (document.querySelector('.modal-backdrop')) return // a dialog inside the Editor owns Esc
    closeNote()
  }

  function handleNoteCreated(doc) {
    newNoteOpen = false
    window.location.hash = `/notas/${doc.id}`
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="board" bind:clientWidth={width}>
  <div class="board-head">
    <h1>Mural de Notas</h1>
    <span class="muted">{$notes.length} {$notes.length === 1 ? 'nota' : 'notas'}</span>
    <button class="btn btn-primary new-btn" onclick={() => newNoteOpen = true}>+ Nova Nota</button>
  </div>

  {#if $notes.length === 0}
    <div class="empty">
      {#if filtered}
        <div class="empty-icon">🔎</div>
        <p><strong>Nenhuma Nota com este filtro.</strong></p>
        <p class="muted">
          {#if $tagFilter.length}Tags: {$tagFilter.join(', ')}{/if}{#if $tagFilter.length && $favoriteFilter} · {/if}{#if $favoriteFilter}só favoritas{/if}
        </p>
        <button class="btn" onclick={() => loadTree([], false)}>Limpar filtros</button>
      {:else}
        <div class="empty-icon">🗒️</div>
        <p><strong>Ainda não há Notas.</strong></p>
        <p class="muted">Uma Nota guarda uma informação rápida: um endereço, um contato, uma lista.</p>
        <button class="btn btn-primary" onclick={() => newNoteOpen = true}>+ Nova Nota</button>
      {/if}
    </div>
  {:else}
    <div class="masonry" style="column-count: {cols}">
      {#each visible as n (n.id)}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <div class="card" role="button" tabindex="0"
             onclick={e => openNote(e, n.id)}
             onkeydown={e => e.key === 'Enter' && e.target === e.currentTarget && openNote(e, n.id)}>
          <strong class="title">{n.is_favorite ? '⭐ ' : ''}{n.title || 'Sem título'}</strong>
          <!-- body_html is sanitized by the server on every write (SanitizeEditorHTML) -->
          <!-- ponytail: empty body_html = 🔒; NoteListItem has no `encrypted` flag, so an empty Nota shows 🔒 too. Add the flag to GET /api/notes if that confuses. -->
          <div class="body">{#if n.body_html}{@html n.body_html}{:else}🔒{/if}</div>
          <div class="foot">
            {#if n.tags?.length}
              <span class="tags">{#each n.tags as t}<span class="chip">#{t}</span>{/each}</span>
            {/if}
            <span class="date">{relDate(n.created_at)}</span>
          </div>
        </div>
      {/each}
    </div>
    {#if shown < $notes.length}<div bind:this={sentinel} class="sentinel"></div>{/if}
  {/if}
</div>

{#if openId}
  <div class="note-backdrop" onclick={closeNote} role="presentation">
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="note-modal" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Nota" tabindex="-1">
      <button class="note-close" onclick={closeNote} title="Fechar" aria-label="Fechar">✕</button>
      {#key openId}<Editor docId={openId} noteModal={true} />{/key}
    </div>
  </div>
{/if}

{#if newNoteOpen}
  <NewNoteDialog onClose={() => newNoteOpen = false} onCreated={handleNoteCreated} />
{/if}

<style>
  .board { flex: 1; padding: 1.25rem 1.5rem 2rem; overflow-y: auto; height: 100%; }
  .board-head { display: flex; align-items: baseline; gap: .75rem; margin-bottom: 1rem; }
  .board-head h1 { font-size: 1.3rem; }
  .new-btn { margin-left: auto; }
  .muted { color: var(--text-muted); }

  .masonry { column-gap: 1rem; }
  .card {
    display: inline-flex; flex-direction: column; gap: .4rem; width: 100%;
    break-inside: avoid; margin-bottom: 1rem;
    background: var(--bg-panel); border: 1px solid var(--border); border-radius: 10px;
    padding: .9rem 1rem; cursor: pointer; color: var(--text);
  }
  .card:hover, .card:focus-visible { border-color: var(--accent); box-shadow: 0 2px 10px rgba(0,0,0,.08); outline: none; }
  .title { font-size: .98rem; overflow-wrap: anywhere; }
  .body { font-size: .88rem; line-height: 1.5; overflow-wrap: anywhere; }
  .body :global(p) { margin: 0 0 .3rem; }
  .body :global(ul), .body :global(ol) { padding-left: 1.1rem; margin: 0 0 .3rem; }
  .body :global(img) { max-width: 100%; border-radius: 6px; }
  .body :global(a) { color: var(--accent); }
  .foot { display: flex; align-items: center; gap: .5rem; justify-content: space-between; margin-top: .2rem; }
  .tags { display: flex; flex-wrap: wrap; gap: .3rem; }
  .chip { font-size: .72rem; padding: .1rem .45rem; border-radius: 999px; background: var(--bg-active); color: var(--text-active); }
  .date { font-size: .75rem; color: var(--text-muted); white-space: nowrap; margin-left: auto; }
  .sentinel { height: 1px; }

  .empty { text-align: center; padding: 4rem 1rem; display: flex; flex-direction: column; align-items: center; gap: .4rem; }
  .empty-icon { font-size: 2.5rem; }

  .note-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex; align-items: center; justify-content: center; z-index: 900; }
  .note-modal {
    position: relative; display: flex; width: min(1200px, 94vw); height: 90vh;
    background: var(--bg); border-radius: 10px; overflow: auto;
  }
  .note-close {
    position: absolute; top: .5rem; right: .75rem; z-index: 5;
    background: var(--bg-panel); border: 1px solid var(--border); border-radius: 6px;
    color: var(--text); cursor: pointer; padding: .15rem .5rem;
  }
  @media (max-width: 640px) {
    .note-modal { width: 100vw; height: 100dvh; border-radius: 0; }
  }
</style>
