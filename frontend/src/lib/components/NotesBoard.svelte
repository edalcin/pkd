<script>
  // Mural de Notas (Spec #8): every Nota as a card, in a grid read row by row,
  // in the server's order (favorites first, then created_at desc). A click
  // opens the Nota in the normal Editor at #/doc/{id}. The sidebar's
  // tag/favorite filters drive the list (Sidebar.svelte → loadNotes).
  import { notes, loadNotes } from '../stores/notes.js'
  import { tagFilter, favoriteFilter, loadTree } from '../stores/documents.js'
  import NewNoteDialog from './NewNoteDialog.svelte'

  loadNotes() // refresh on every visit: the Nota may have changed in the Editor

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

  function openNote(id) {
    window.location.hash = `/doc/${id}`
  }

  // Card preview: plain-text excerpt + first image as thumbnail, so all cards
  // have about the same height. body_html is sanitized by the server.
  const parser = new DOMParser()
  function preview(html) {
    const d = parser.parseFromString(html, 'text/html')
    // One space between blocks, so "<p>a</p><p>b</p>" reads "a b", not "ab".
    const text = Array.from(d.body.children, c => c.textContent.trim()).filter(Boolean).join(' ')
    return { text, thumb: d.querySelector('img')?.getAttribute('src') || null }
  }

  function handleNoteCreated(doc) {
    newNoteOpen = false
    window.location.hash = `/doc/${doc.id}`
  }
</script>

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
    <div class="grid" style="grid-template-columns: repeat({cols}, minmax(0, 1fr))">
      {#each visible as n (n.id)}
        <div class="card" role="button" tabindex="0"
             onclick={() => openNote(n.id)}
             onkeydown={e => e.key === 'Enter' && openNote(n.id)}>
          <strong class="title">{n.is_favorite ? '⭐ ' : ''}{n.title || 'Sem título'}</strong>
          <!-- ponytail: empty body_html = 🔒; NoteListItem has no `encrypted` flag, so an empty Nota shows 🔒 too. Add the flag to GET /api/notes if that confuses. -->
          {#if n.body_html}
            {@const p = preview(n.body_html)}
            <div class="body">
              {#if p.thumb}<img class="thumb" src={p.thumb} alt="" loading="lazy" />{/if}
              <p class="excerpt">{p.text}</p>
            </div>
          {:else}<div class="body">🔒</div>{/if}
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

{#if newNoteOpen}
  <NewNoteDialog onClose={() => newNoteOpen = false} onCreated={handleNoteCreated} />
{/if}

<style>
  .board { flex: 1; padding: 1.25rem 1.5rem 2rem; overflow-y: auto; height: 100%; }
  .board-head { display: flex; align-items: baseline; gap: .75rem; margin-bottom: 1rem; }
  .board-head h1 { font-size: 1.3rem; }
  .new-btn { margin-left: auto; }
  .muted { color: var(--text-muted); }

  .grid { display: grid; gap: 1rem; }
  .card {
    display: flex; flex-direction: column; gap: .4rem; height: 11rem;
    background: var(--bg-panel); border: 1px solid var(--border); border-radius: 10px;
    padding: .9rem 1rem; cursor: pointer; color: var(--text); overflow: hidden;
  }
  .card:hover, .card:focus-visible { border-color: var(--accent); box-shadow: 0 2px 10px rgba(0,0,0,.08); outline: none; }
  .title { font-size: .98rem; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; flex-shrink: 0; }
  .body { flex: 1; min-height: 0; overflow: hidden; font-size: .88rem; line-height: 1.5; }
  .thumb { float: left; width: 64px; height: 64px; object-fit: cover; border-radius: 6px; margin-right: .6rem; }
  .excerpt { margin: 0; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
  .foot { display: flex; align-items: center; gap: .5rem; justify-content: space-between; margin-top: .2rem; }
  .tags { display: flex; flex-wrap: wrap; gap: .3rem; }
  .chip { font-size: .72rem; padding: .1rem .45rem; border-radius: 999px; background: var(--bg-active); color: var(--text-active); }
  .date { font-size: .75rem; color: var(--text-muted); white-space: nowrap; margin-left: auto; }
  .sentinel { height: 1px; }

  .empty { text-align: center; padding: 4rem 1rem; display: flex; flex-direction: column; align-items: center; gap: .4rem; }
  .empty-icon { font-size: 2.5rem; }

</style>
