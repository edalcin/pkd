<script>
  import { onMount } from 'svelte'

  // Module-level cache so the Boxicons CSS is only fetched once per page load.
  let iconCache = null

  const ICONS = [
    'bx-file-blank', 'bxs-file', 'bx-file',
    'bx-note', 'bxs-note',
    'bx-clipboard', 'bxs-clipboard',
    'bx-notepad',
    'bx-list-ul', 'bx-list-ol',
    'bx-book', 'bxs-book',
    'bx-book-open', 'bxs-book-open',
    'bx-bookmark', 'bxs-bookmark',
    'bx-folder', 'bxs-folder',
    'bx-folder-open', 'bxs-folder-open',
    'bx-archive', 'bxs-archive',
    'bx-home', 'bxs-home',
    'bx-building', 'bxs-building',
    'bx-map', 'bxs-map',
    'bx-map-pin', 'bxs-map-pin',
    'bx-user', 'bxs-user',
    'bx-group', 'bxs-group',
    'bx-star', 'bxs-star',
    'bx-heart', 'bxs-heart',
    'bx-tag', 'bxs-tag',
    'bx-flag', 'bxs-flag',
    'bx-pin', 'bxs-pin',
    'bx-check-circle', 'bxs-check-circle',
    'bx-calendar', 'bxs-calendar',
    'bx-time', 'bxs-time',
    'bx-alarm', 'bxs-alarm',
    'bx-pencil', 'bxs-pencil',
    'bx-edit', 'bxs-edit',
    'bx-bulb', 'bxs-bulb',
    'bx-brain', 'bxs-brain',
    'bx-graduation-cap', 'bxs-graduation-cap',
    'bx-atom',
    'bx-code', 'bx-code-block',
    'bx-terminal',
    'bx-chip', 'bxs-chip',
    'bx-server', 'bxs-server',
    'bx-desktop', 'bxs-desktop',
    'bx-laptop', 'bxs-laptop',
    'bx-wifi', 'bxs-wifi',
    'bx-network-chart',
    'bx-globe', 'bxs-globe',
    'bx-link', 'bx-link-external',
    'bx-message', 'bxs-message',
    'bx-envelope', 'bxs-envelope',
    'bx-bell', 'bxs-bell',
    'bx-phone', 'bxs-phone',
    'bx-image', 'bxs-image',
    'bx-camera', 'bxs-camera',
    'bx-music', 'bxs-music',
    'bx-film', 'bxs-film',
    'bx-cog', 'bxs-cog',
    'bx-wrench',
    'bx-shield', 'bxs-shield',
    'bx-lock', 'bxs-lock',
    'bx-key', 'bxs-key',
    'bx-search', 'bxs-search',
    'bx-briefcase', 'bxs-briefcase',
    'bx-bar-chart-alt', 'bxs-bar-chart-alt',
    'bx-leaf', 'bxs-leaf',
    'bx-sun', 'bxs-sun',
    'bx-moon', 'bxs-moon',
    'bx-package', 'bxs-package',
    'bx-trophy', 'bxs-trophy',
    'bx-gift', 'bxs-gift',
    'bx-restaurant',
    'bx-dish',
    'bx-basket', 'bxs-basket',
    'bx-dollar-circle', 'bxs-dollar-circle',
    'bx-trending-up',
    'bx-paint', 'bxs-paint',
    'bx-run',
    'bx-walk',
    'bx-dumbbell',
    'bx-car', 'bxs-car',
    'bx-plane', 'bxs-plane',
    'bx-train', 'bxs-train',
    'bx-dock-top',
  ]

  let { value = '', onSelect, onClose } = $props()
  let query = $state('')
  let customIcon = $state('')
  let allIcons = $state(ICONS)

  onMount(async () => {
    if (iconCache) {
      allIcons = iconCache
      return
    }
    try {
      const res = await fetch('https://unpkg.com/boxicons@2.1.4/css/boxicons.min.css')
      const css = await res.text()
      const matches = []
      const re = /\.(bx[sl]?-[\w-]+)::before/g
      let m
      while ((m = re.exec(css)) !== null) {
        matches.push(m[1])
      }
      if (matches.length > 0) {
        iconCache = [...new Set(matches)]
        allIcons = iconCache
      }
    } catch {
      // Network unavailable — keep the curated list
    }
  })

  const filtered = $derived(
    query.trim()
      ? allIcons.filter(ic => ic.includes(query.trim().toLowerCase()))
      : ICONS
  )

  function select(icon) {
    onSelect(icon)
    onClose()
  }

  function applyCustom() {
    const ic = customIcon.trim()
    if (ic) { onSelect(ic); onClose() }
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) onClose()
  }
</script>

<div class="picker-backdrop" onmousedown={handleBackdropClick} role="presentation">
  <div class="icon-picker" role="dialog" aria-label="Selecionar ícone">
    <div class="picker-header">
      <input
        class="icon-search"
        type="text"
        placeholder="Buscar ícone…"
        bind:value={query}
        autofocus
        onkeydown={e => e.key === 'Escape' && onClose()}
      />
      {#if value}
        <button class="remove-btn" onclick={() => { onSelect(''); onClose() }}>✕ Remover</button>
      {/if}
    </div>
    <div class="icon-grid" role="listbox">
      {#each filtered as icon}
        <button
          class="icon-item {icon === value ? 'selected' : ''}"
          onclick={() => select(icon)}
          title={icon}
          role="option"
          aria-selected={icon === value}
        >
          <i class="bx {icon}"></i>
        </button>
      {/each}
      {#if filtered.length === 0}
        <p class="no-results">Nenhum ícone encontrado</p>
      {/if}
    </div>
    <div class="custom-row">
      <span class="custom-label">Nome do ícone:</span>
      <input
        class="custom-input"
        type="text"
        placeholder="ex: bx-home"
        bind:value={customIcon}
        onkeydown={e => e.key === 'Enter' && applyCustom()}
      />
      <button class="custom-apply" onclick={applyCustom} disabled={!customIcon.trim()}>Aplicar</button>
    </div>
  </div>
</div>

<style>
  .picker-backdrop {
    position: fixed;
    inset: 0;
    z-index: 400;
    background: transparent;
  }

  .icon-picker {
    position: fixed;
    background: var(--bg-panel, #fff);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 8px 32px rgba(0,0,0,.22);
    padding: .5rem;
    width: 320px;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    z-index: 401;
    display: flex;
    flex-direction: column;
    gap: .4rem;
  }

  .picker-header {
    display: flex;
    gap: .4rem;
    align-items: center;
  }

  .icon-search {
    flex: 1;
    padding: .3rem .5rem;
    border: 1px solid var(--border);
    border-radius: 5px;
    font-size: .8rem;
    background: var(--bg);
    color: var(--text);
    min-width: 0;
  }
  .icon-search:focus { outline: none; border-color: var(--accent); }

  .remove-btn {
    font-size: .75rem;
    color: var(--text-muted);
    padding: .25rem .4rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    background: none;
  }
  .remove-btn:hover { color: var(--danger, #dc2626); border-color: var(--danger, #dc2626); }

  .icon-grid {
    display: grid;
    grid-template-columns: repeat(9, 1fr);
    gap: 2px;
    max-height: 260px;
    overflow-y: auto;
    scrollbar-width: thin;
  }

  .icon-item {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    border-radius: 5px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    border: 1px solid transparent;
    background: none;
    transition: background .1s;
  }
  .icon-item:hover { background: var(--bg-hover); border-color: var(--border); }
  .icon-item.selected { background: var(--accent); color: #fff; border-color: var(--accent); }
  .icon-item i { pointer-events: none; }

  .no-results {
    grid-column: 1 / -1;
    color: var(--text-muted);
    font-size: .8rem;
    text-align: center;
    padding: 1rem 0;
  }

  .custom-row {
    display: flex;
    align-items: center;
    gap: .35rem;
    border-top: 1px solid var(--border);
    padding-top: .4rem;
  }

  .custom-label {
    font-size: .7rem;
    color: var(--text-muted);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .custom-input {
    flex: 1;
    padding: .2rem .4rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    font-size: .75rem;
    background: var(--bg);
    color: var(--text);
    min-width: 0;
  }
  .custom-input:focus { outline: none; border-color: var(--accent); }

  .custom-apply {
    padding: .2rem .5rem;
    border: 1px solid var(--accent);
    border-radius: 4px;
    background: none;
    color: var(--accent);
    font-size: .75rem;
    cursor: pointer;
    flex-shrink: 0;
  }
  .custom-apply:disabled { opacity: .4; cursor: not-allowed; }
  .custom-apply:not(:disabled):hover { background: var(--accent); color: #fff; }
</style>
