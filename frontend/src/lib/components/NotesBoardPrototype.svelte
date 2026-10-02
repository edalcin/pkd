<script>
  // PROTOTYPE — throwaway (wayfinder ticket #2 "Conteúdo do modal da Nota").
  // Three structurally different ways to open a Nota from the Mural de Notas,
  // switchable via #/notas-prototype?variant=A|B|C and the floating bar.
  // Lives on branch prototype/mural-notas-modal only; never merge to main.
  import { notes, loadNotes } from '../stores/notes.js'
  import Editor from './Editor.svelte'

  const VARIANTS = {
    A: 'Editor completo no modal',
    B: 'Subconjunto no modal',
    C: 'Painel lateral (drawer)',
  }
  const keys = Object.keys(VARIANTS)

  function readVariant() {
    const q = window.location.hash.split('?')[1] || ''
    const v = new URLSearchParams(q).get('variant')
    return keys.includes(v) ? v : 'A'
  }
  let variant = $state(readVariant())
  let openId = $state(null)
  let expanded = $state(false) // C: drawer → full Editor

  function setVariant(v) {
    variant = v
    openId = null
    expanded = false
    history.replaceState(null, '', `#/notas-prototype?variant=${v}`)
  }
  function cycle(step) {
    const i = keys.indexOf(variant)
    setVariant(keys[(i + step + keys.length) % keys.length])
  }
  function onKey(e) {
    const t = e.target
    if (t.closest?.('input, textarea, [contenteditable="true"]')) return
    if (e.key === 'ArrowLeft') cycle(-1)
    if (e.key === 'ArrowRight') cycle(1)
    if (e.key === 'Escape') close()
  }
  function close() { openId = null; expanded = false; loadNotes() }

  $effect(() => { loadNotes() })

  const rtf = new Intl.RelativeTimeFormat('pt-BR', { numeric: 'auto' })
  function relDate(iso) {
    const s = (new Date(iso) - Date.now()) / 1000
    const units = [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]]
    for (const [u, sec] of units) if (Math.abs(s) >= sec) return rtf.format(Math.round(s / sec), u)
    return 'agora'
  }
  function preview(html) {
    const el = document.createElement('div')
    el.innerHTML = html || ''
    return (el.textContent || '').trim()
  }
  const open = $derived($notes.find(n => n.id === openId))
  const stub = what => alert(`PROTOTYPE: ${what} (sem efeito)`)
</script>

<svelte:window onkeydown={onKey} />

<div class="board" class:with-drawer={variant === 'C' && openId && !expanded}>
  <div class="board-head">
    <h1>Mural de Notas</h1>
    <span class="muted">{$notes.length} notas</span>
    <button class="new-btn" onclick={() => stub('+ Nova Nota')}>+ Nova Nota</button>
  </div>

  <div class="grid">
    {#each $notes as n (n.id)}
      <button class="card" class:active={n.id === openId} onclick={() => { openId = n.id; expanded = false }}>
        <span class="date">{relDate(n.created_at)}{n.is_favorite ? ' · ⭐' : ''}</span>
        <strong class="title">{n.title}</strong>
        {#if n.body_html}
          <span class="prev">{preview(n.body_html)}</span>
        {:else}
          <span class="prev muted">🔒 / vazio</span>
        {/if}
        {#if n.tags?.length}
          <span class="tags">{#each n.tags as t}<span class="chip">#{t}</span>{/each}</span>
        {/if}
      </button>
    {/each}
  </div>
</div>

<!-- A: full Editor inside a large modal -->
{#if variant === 'A' && openId}
  <div class="backdrop" onclick={close} role="presentation">
    <div class="modal-a" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <button class="x" onclick={close}>✕</button>
      {#key openId}<Editor docId={openId} />{/key}
    </div>
  </div>
{/if}

<!-- B: subset modal — title, body, tags, actions; "abrir como documento" escape hatch -->
{#if variant === 'B' && open}
  <div class="backdrop" onclick={close} role="presentation">
    <div class="modal-b" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <div class="b-head">
        <span class="muted">{relDate(open.created_at)}</span>
        <button class="x-inline" onclick={close}>✕</button>
      </div>
      <h2 contenteditable="true">{open.title}</h2>
      <div class="b-body" contenteditable="true">{@html open.body_html}</div>
      <div class="tags">
        {#each open.tags || [] as t}<span class="chip">#{t} ✕</span>{/each}
        <span class="chip add">+ tag</span>
      </div>
      <div class="b-actions">
        <button onclick={() => stub('Converter em Memória')}>Converter em Memória</button>
        <button onclick={() => stub('Converter em Documento')}>Converter em Documento</button>
        <a href={`#/doc/${open.id}`}>Abrir como documento ↗</a>
        <span class="spacer"></span>
        <button class="danger" onclick={() => stub('Deletar')}>🗑 Deletar</button>
      </div>
      <p class="muted small">Sem anexos, links externos, histórico, lock/cifra, data. Para isso: "Abrir como documento".</p>
    </div>
  </div>
{/if}

<!-- C: right drawer with the subset; "expandir" swaps to the full Editor in a modal -->
{#if variant === 'C' && open && !expanded}
  <aside class="drawer">
    <div class="b-head">
      <span class="muted">{relDate(open.created_at)}</span>
      <button onclick={() => (expanded = true)}>⤢ Expandir</button>
      <button class="x-inline" onclick={close}>✕</button>
    </div>
    <h2 contenteditable="true">{open.title}</h2>
    <div class="b-body" contenteditable="true">{@html open.body_html}</div>
    <div class="tags">
      {#each open.tags || [] as t}<span class="chip">#{t} ✕</span>{/each}
      <span class="chip add">+ tag</span>
    </div>
    <div class="b-actions">
      <button class="danger" onclick={() => stub('Deletar')}>🗑 Deletar</button>
    </div>
  </aside>
{/if}
{#if variant === 'C' && openId && expanded}
  <div class="backdrop" onclick={close} role="presentation">
    <div class="modal-a" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <button class="x" onclick={close}>✕</button>
      {#key openId}<Editor docId={openId} />{/key}
    </div>
  </div>
{/if}

<div class="switcher">
  <button onclick={() => cycle(-1)}>◀</button>
  <span>{variant} ({VARIANTS[variant]})</span>
  <button onclick={() => cycle(1)}>▶</button>
</div>

<style>
  .board { padding: 1.25rem 1.5rem 5rem; overflow-y: auto; height: 100%; }
  .board.with-drawer { margin-right: 420px; }
  .board-head { display: flex; align-items: baseline; gap: .75rem; margin-bottom: 1rem; }
  .board-head h1 { font-size: 1.3rem; }
  .new-btn { margin-left: auto; }
  .muted { color: var(--text-muted); }
  .small { font-size: .8rem; margin-top: .75rem; }
  .grid { display: grid; gap: 1rem; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); }
  @media (min-width: 1400px) { .grid { grid-template-columns: repeat(4, 1fr); } }
  .card {
    display: flex; flex-direction: column; gap: .4rem; text-align: left;
    background: var(--bg-panel); border: 1px solid var(--border); border-radius: 10px;
    padding: .9rem 1rem; min-height: 120px; cursor: pointer; color: var(--text); font: inherit;
  }
  .card:hover, .card.active { border-color: var(--accent); box-shadow: 0 2px 10px rgba(0,0,0,.08); }
  .date { font-size: .75rem; color: var(--text-muted); }
  .title { font-size: .98rem; }
  .prev { font-size: .88rem; display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
  .tags { display: flex; flex-wrap: wrap; gap: .3rem; }
  .chip { font-size: .72rem; padding: .1rem .45rem; border-radius: 999px; background: var(--bg-active); color: var(--text-active); }
  .chip.add { background: transparent; border: 1px dashed var(--border); color: var(--text-muted); }
  .backdrop { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex; align-items: center; justify-content: center; z-index: 900; }
  .modal-a { position: relative; width: min(1200px, 94vw); height: 90vh; background: var(--bg); border-radius: 10px; overflow: auto; }
  .modal-b { width: min(680px, 94vw); max-height: 88vh; overflow: auto; background: var(--bg-panel); border-radius: 10px; padding: 1.25rem 1.5rem; }
  .x { position: absolute; top: .5rem; right: .75rem; z-index: 5; }
  .b-head { display: flex; align-items: center; gap: .5rem; margin-bottom: .5rem; }
  .x-inline { margin-left: auto; }
  .b-head button:first-of-type:not(.x-inline) { margin-left: auto; }
  .b-head button + .x-inline { margin-left: 0; }
  h2 { font-size: 1.2rem; margin-bottom: .6rem; outline: none; }
  .b-body { min-height: 120px; outline: none; margin-bottom: .9rem; line-height: 1.55; }
  .b-actions { display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; margin-top: 1rem; border-top: 1px solid var(--border); padding-top: .75rem; }
  .spacer { flex: 1; }
  .danger { color: #cf222e; }
  .drawer { position: fixed; top: 48px; right: 0; bottom: 0; width: 420px; background: var(--bg-panel); border-left: 1px solid var(--border); padding: 1rem 1.25rem; overflow: auto; z-index: 800; }
  .switcher {
    position: fixed; bottom: 16px; left: 50%; transform: translateX(-50%); z-index: 1000;
    display: flex; gap: .6rem; align-items: center; padding: .4rem .8rem;
    background: #111; color: #fff; border-radius: 999px; box-shadow: 0 4px 16px rgba(0,0,0,.35); font-size: .85rem;
  }
  .switcher button { background: none; border: none; color: #fff; cursor: pointer; font-size: 1rem; }
</style>
