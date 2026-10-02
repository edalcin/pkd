<script>
  // PROTOTYPE — throwaway (wayfinder ticket #6 "Visual do card, breakpoints
  // responsivos e estado vazio do Mural"). Three structurally different card
  // layouts, switchable via #/notas-prototype?variant=1|2|3 and the floating
  // bar. "Vazio" toggles the empty state. Modal = full Editor (decided in #2).
  // Lives on branch prototype/mural-notas-modal only; never merge to main.
  import { notes, loadNotes } from '../stores/notes.js'
  import { tagFilter, favoriteFilter } from '../stores/documents.js'
  import Editor from './Editor.svelte'

  const VARIANTS = {
    1: 'Grade uniforme (referência)',
    2: 'Masonry (corpo inteiro)',
    3: 'Compacto (título + 1 linha)',
  }
  const keys = Object.keys(VARIANTS)

  function readVariant() {
    const q = window.location.hash.split('?')[1] || ''
    const v = new URLSearchParams(q).get('variant')
    return keys.includes(v) ? v : '1'
  }
  let variant = $state(readVariant())
  let openId = $state(null)
  let forceEmpty = $state(false)
  let width = $state(0)

  function setVariant(v) {
    variant = v
    history.replaceState(null, '', `#/notas-prototype?variant=${v}`)
  }
  function cycle(step) {
    const i = keys.indexOf(variant)
    setVariant(keys[(i + step + keys.length) % keys.length])
  }
  function onKey(e) {
    if (e.target.closest?.('input, textarea, [contenteditable="true"]')) return
    if (e.key === 'ArrowLeft') cycle(-1)
    if (e.key === 'ArrowRight') cycle(1)
    if (e.key === 'Escape') { openId = null; loadNotes() }
  }

  $effect(() => { loadNotes() })

  const rtf = new Intl.RelativeTimeFormat('pt-BR', { numeric: 'auto' })
  function relDate(iso) {
    const s = (new Date(iso) - Date.now()) / 1000
    const units = [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]]
    for (const [u, sec] of units) if (Math.abs(s) >= sec) return rtf.format(Math.round(s / sec), u)
    return 'agora'
  }
  function text(html) {
    const el = document.createElement('div')
    el.innerHTML = html || ''
    return (el.textContent || '').trim()
  }
  const list = $derived(forceEmpty ? [] : $notes)
  const filtered = $derived($tagFilter.length > 0 || $favoriteFilter)
  const cols = $derived(width < 640 ? 1 : width < 960 ? 2 : width < 1280 ? 3 : 4)
</script>

<svelte:window onkeydown={onKey} />

<div class="board" bind:clientWidth={width}>
  <div class="board-head">
    <h1>Mural de Notas</h1>
    <span class="muted">{list.length} notas · largura {width}px → {cols} col.</span>
    <button class="new-btn">+ Nova Nota</button>
  </div>

  {#if list.length === 0}
    <div class="empty">
      {#if filtered}
        <div class="empty-icon">🔎</div>
        <p><strong>Nenhuma Nota com este filtro.</strong></p>
        <p class="muted">Tags: {$tagFilter.join(', ') || '—'}{$favoriteFilter ? ' · só favoritas' : ''}</p>
        <button>Limpar filtros</button>
      {:else}
        <div class="empty-icon">🗒️</div>
        <p><strong>Ainda não há Notas.</strong></p>
        <p class="muted">Uma Nota guarda uma informação rápida: um endereço, um contato, uma lista.</p>
        <button>+ Nova Nota</button>
      {/if}
    </div>
  {:else if variant === '1'}
    <!-- 1: uniform grid, fixed-height cards, 4-line text preview, chips at bottom -->
    <div class="grid v1" style="grid-template-columns: repeat({cols}, 1fr)">
      {#each list as n (n.id)}
        <button class="card c1" onclick={() => (openId = n.id)}>
          <span class="date">{relDate(n.created_at)}{n.is_favorite ? ' · ⭐' : ''}</span>
          <strong class="title">{n.title}</strong>
          <span class="prev clamp4">{text(n.body_html) || '🔒'}</span>
          {#if n.tags?.length}<span class="tags">{#each n.tags as t}<span class="chip">#{t}</span>{/each}</span>{/if}
        </button>
      {/each}
    </div>
  {:else if variant === '2'}
    <!-- 2: masonry (CSS columns), variable height, rendered HTML body (Notas are short: median 66 chars) -->
    <div class="masonry" style="column-count: {cols}">
      {#each list as n (n.id)}
        <button class="card c2" onclick={() => (openId = n.id)}>
          <strong class="title">{n.is_favorite ? '⭐ ' : ''}{n.title}</strong>
          <div class="body-html">{#if n.body_html}{@html n.body_html}{:else}🔒{/if}</div>
          <span class="foot">
            {#if n.tags?.length}<span class="tags">{#each n.tags as t}<span class="chip">#{t}</span>{/each}</span>{/if}
            <span class="date">{relDate(n.created_at)}</span>
          </span>
        </button>
      {/each}
    </div>
  {:else}
    <!-- 3: compact tiles, title + one line, date right, tags as dots; denser (more per row) -->
    <div class="grid v3" style="grid-template-columns: repeat({Math.min(cols + 1, 5)}, 1fr)">
      {#each list as n (n.id)}
        <button class="card c3" onclick={() => (openId = n.id)}>
          <span class="row"><strong class="title one">{n.is_favorite ? '⭐ ' : ''}{n.title}</strong><span class="date">{relDate(n.created_at)}</span></span>
          <span class="prev one muted">{text(n.body_html) || '🔒'}</span>
          {#if n.tags?.length}<span class="dots">{#each n.tags as t}<span title={t}>#{t}</span>{/each}</span>{/if}
        </button>
      {/each}
    </div>
  {/if}
</div>

{#if openId}
  <div class="backdrop" onclick={() => { openId = null; loadNotes() }} role="presentation">
    <div class="modal-a" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <button class="x" onclick={() => { openId = null; loadNotes() }}>✕</button>
      {#key openId}<Editor docId={openId} />{/key}
    </div>
  </div>
{/if}

<div class="switcher">
  <button onclick={() => cycle(-1)}>◀</button>
  <span>{variant} ({VARIANTS[variant]})</span>
  <button onclick={() => cycle(1)}>▶</button>
  <label><input type="checkbox" bind:checked={forceEmpty} /> vazio</label>
</div>

<style>
  .board { padding: 1.25rem 1.5rem 5rem; overflow-y: auto; height: 100%; }
  .board-head { display: flex; align-items: baseline; gap: .75rem; margin-bottom: 1rem; }
  .board-head h1 { font-size: 1.3rem; }
  .new-btn { margin-left: auto; }
  .muted { color: var(--text-muted); }
  .grid { display: grid; gap: 1rem; }
  .card {
    display: flex; flex-direction: column; gap: .4rem; text-align: left; width: 100%;
    background: var(--bg-panel); border: 1px solid var(--border); border-radius: 10px;
    padding: .9rem 1rem; cursor: pointer; color: var(--text); font: inherit;
  }
  .card:hover { border-color: var(--accent); box-shadow: 0 2px 10px rgba(0,0,0,.08); }
  .date { font-size: .75rem; color: var(--text-muted); }
  .title { font-size: .98rem; }
  .tags { display: flex; flex-wrap: wrap; gap: .3rem; }
  .chip { font-size: .72rem; padding: .1rem .45rem; border-radius: 999px; background: var(--bg-active); color: var(--text-active); }
  /* 1 */
  .c1 { height: 170px; overflow: hidden; }
  .c1 .tags { margin-top: auto; }
  .clamp4 { font-size: .88rem; display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
  /* 2 */
  .masonry { column-gap: 1rem; }
  .c2 { break-inside: avoid; margin-bottom: 1rem; display: inline-flex; }
  .body-html { font-size: .88rem; line-height: 1.5; overflow-wrap: anywhere; }
  .body-html :global(p) { margin: 0 0 .3rem; }
  .body-html :global(ul) { padding-left: 1.1rem; }
  .foot { display: flex; align-items: center; gap: .5rem; justify-content: space-between; margin-top: .3rem; }
  /* 3 */
  .v3 { gap: .6rem; }
  .c3 { padding: .55rem .75rem; gap: .2rem; border-radius: 8px; }
  .row { display: flex; gap: .5rem; align-items: baseline; }
  .row .date { margin-left: auto; white-space: nowrap; }
  .one { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; font-size: .85rem; }
  .dots { display: flex; gap: .4rem; font-size: .7rem; color: var(--text-active); }
  /* empty */
  .empty { text-align: center; padding: 4rem 1rem; display: flex; flex-direction: column; align-items: center; gap: .4rem; }
  .empty-icon { font-size: 2.5rem; }
  /* modal */
  .backdrop { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex; align-items: center; justify-content: center; z-index: 900; }
  .modal-a { position: relative; width: min(1200px, 94vw); height: 90vh; background: var(--bg); border-radius: 10px; overflow: auto; }
  .x { position: absolute; top: .5rem; right: .75rem; z-index: 5; }
  .switcher {
    position: fixed; bottom: 16px; left: 50%; transform: translateX(-50%); z-index: 1000;
    display: flex; gap: .6rem; align-items: center; padding: .4rem .8rem;
    background: #111; color: #fff; border-radius: 999px; box-shadow: 0 4px 16px rgba(0,0,0,.35); font-size: .85rem;
  }
  .switcher button { background: none; border: none; color: #fff; cursor: pointer; font-size: 1rem; }
</style>
