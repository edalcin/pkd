<script>
  import { onMount } from 'svelte'
  import { authenticated, checkSession, logout } from './lib/stores/auth.js'
  import { loadTree, textFilter, tagFilter, favoriteFilter, revealActiveSignal, activeDoc } from './lib/stores/documents.js'
  import { loadTags } from './lib/stores/tags.js'
  import LoginPage from './lib/components/LoginPage.svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import Editor from './lib/components/Editor.svelte'
  import GraphView from './lib/components/GraphView.svelte'
  import Calendar from './lib/components/Calendar.svelte'
  import Admin from './lib/components/Admin.svelte'
  import Chat from './lib/components/Chat.svelte'
  import ShareDialog from './lib/components/ShareDialog.svelte'
  import { apiGet } from './lib/api.js'

  // ─── Routing ─────────────────────────────────────────────
  let hash = $state(window.location.hash.slice(1) || '/')

  // ─── Navigation history ──────────────────────────────────
  let navHistory = $state([window.location.hash.slice(1) || '/'])
  let navPos = $state(0)
  let suppressHistoryPush = false

  const canGoBack = $derived(navPos > 0)
  const canGoForward = $derived(navPos < navHistory.length - 1)

  window.addEventListener('hashchange', () => {
    const newHash = window.location.hash.slice(1) || '/'
    hash = newHash
    if (newHash !== '/') localStorage.setItem('pkd-last-route', '#' + newHash)
    if (suppressHistoryPush) {
      suppressHistoryPush = false
      return
    }
    if (navHistory[navPos] !== newHash) {
      const trimmed = navHistory.slice(0, navPos + 1)
      navHistory = [...trimmed, newHash]
      navPos = trimmed.length
    }
  })

  function goBack() {
    if (!canGoBack) return
    suppressHistoryPush = true
    navPos--
    window.location.hash = navHistory[navPos]
  }

  function goForward() {
    if (!canGoForward) return
    suppressHistoryPush = true
    navPos++
    window.location.hash = navHistory[navPos]
  }

  $effect(() => {
    function onKeydown(e) {
      if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
        if (e.key === 'ArrowLeft') { e.preventDefault(); goBack() }
        else if (e.key === 'ArrowRight') { e.preventDefault(); goForward() }
      }
    }
    document.addEventListener('keydown', onKeydown)
    return () => document.removeEventListener('keydown', onKeydown)
  })

  // ─── Pake/Tauri: open external links in system browser ───────
  $effect(() => {
    if (window.__TAURI_INTERNALS__ === undefined && window.__TAURI__ === undefined) return
    function handleExternalLink(e) {
      const a = e.target.closest('a[href]')
      if (!a) return
      const href = a.href
      if (!href.startsWith('http://') && !href.startsWith('https://')) return
      try { if (new URL(href).origin === window.location.origin) return } catch { return }
      e.preventDefault()
      e.stopPropagation()
      if (window.__TAURI_INTERNALS__) {
        window.__TAURI_INTERNALS__.invoke('plugin:shell|open', { path: href })
          .catch(() => window.open(href, '_blank', 'noreferrer'))
      } else {
        window.__TAURI__.shell.open(href)
      }
    }
    document.addEventListener('click', handleExternalLink, true)
    return () => document.removeEventListener('click', handleExternalLink, true)
  })

  function getRoute() {
    if (hash.startsWith('/focus/')) return { view: 'focus', id: hash.split('/')[2] }
    if (hash.startsWith('/doc/')) return { view: 'doc', id: hash.split('/')[2] }
    if (hash === '/graph') return { view: 'graph' }
    if (hash === '/calendar') return { view: 'calendar' }
    if (hash === '/admin') return { view: 'admin' }
    if (hash === '/chat') return { view: 'chat' }
    return { view: 'home' }
  }

  // ─── Theme ───────────────────────────────────────────────
  let theme = $state(localStorage.getItem('pkd-theme') || 'light')
  $effect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    localStorage.setItem('pkd-theme', theme)
  })
  function toggleTheme() {
    theme = theme === 'light' ? 'dark' : 'light'
  }

  // ─── Sidebar resize (desktop) ─────────────────────────────
  let sidebarWidth = $state(Number(localStorage.getItem('pkd-sidebar-width')) || 260)
  let resizing = $state(false)
  let resizeAnchor = { x: 0, w: 0 }

  function onResizeStart(e) {
    resizing = true
    resizeAnchor = { x: e.clientX, w: sidebarWidth }
    document.addEventListener('mousemove', onResizeMove)
    document.addEventListener('mouseup', onResizeEnd)
  }

  function onResizeMove(e) {
    const w = Math.max(160, Math.min(480, resizeAnchor.w + e.clientX - resizeAnchor.x))
    sidebarWidth = w
  }

  function onResizeEnd() {
    resizing = false
    localStorage.setItem('pkd-sidebar-width', String(sidebarWidth))
    document.removeEventListener('mousemove', onResizeMove)
    document.removeEventListener('mouseup', onResizeEnd)
  }

  // ─── Panel collapse state ────────────────────────────────────
  let sidebarCollapsed = $state(localStorage.getItem('pkd-sidebar-collapsed') === 'true')
  let assocCollapsed = $state(localStorage.getItem('pkd-assoc-collapsed') === 'true')

  function toggleSidebarCollapse() {
    sidebarCollapsed = !sidebarCollapsed
    localStorage.setItem('pkd-sidebar-collapsed', String(sidebarCollapsed))
  }

  function toggleAssocCollapse() {
    assocCollapsed = !assocCollapsed
    localStorage.setItem('pkd-assoc-collapsed', String(assocCollapsed))
  }

  // ─── Right panel (associations) resize ──────────────────────
  let isMobile = $state(window.matchMedia('(max-width: 640px)').matches)
  let assocSidebarEl = $state(null)
  let assocPanelWidth = $state(Number(localStorage.getItem('pkd-assoc-width')) || 300)
  let resizingAssoc = $state(false)
  let assocResizeAnchor = { x: 0, w: 0 }

  $effect(() => {
    const mq = window.matchMedia('(max-width: 640px)')
    const handler = (e) => { isMobile = e.matches }
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  })

  function onAssocResizeStart(e) {
    resizingAssoc = true
    assocResizeAnchor = { x: e.clientX, w: assocPanelWidth }
    document.addEventListener('mousemove', onAssocResizeMove)
    document.addEventListener('mouseup', onAssocResizeEnd)
  }

  function onAssocResizeMove(e) {
    assocPanelWidth = Math.max(220, Math.min(600, assocResizeAnchor.w + assocResizeAnchor.x - e.clientX))
  }

  function onAssocResizeEnd() {
    resizingAssoc = false
    localStorage.setItem('pkd-assoc-width', String(assocPanelWidth))
    document.removeEventListener('mousemove', onAssocResizeMove)
    document.removeEventListener('mouseup', onAssocResizeEnd)
  }

  // ─── Document filter ──────────────────────────────────────
  let filterTimer = null

  const hasActiveFilters = $derived($textFilter.length > 0 || $tagFilter.length > 0 || $favoriteFilter)

  function onFilterInput(e) {
    const val = e.target.value
    textFilter.set(val)
    clearTimeout(filterTimer)
    filterTimer = setTimeout(() => loadTree(undefined, undefined, val), 200)
  }

  async function clearFilter() {
    clearTimeout(filterTimer)
    textFilter.set('')
    await loadTree(undefined, undefined, '')
    revealActiveSignal.update(n => n + 1)
  }

  async function resetFilters() {
    clearTimeout(filterTimer)
    textFilter.set('')
    await loadTree([], false, '')
  }

  // ─── Share dialog ─────────────────────────────────────────
  let shareDocId = $state(null)
  function openShare(id) { shareDocId = id }
  function closeShare() { shareDocId = null }

  // ─── Bootstrap ───────────────────────────────────────────
  function restoreLastRoute() {
    const h = window.location.hash
    if (!h || h === '#' || h === '#/') {
      const saved = localStorage.getItem('pkd-last-route')
      if (saved) window.location.hash = saved
    }
  }

  onMount(async () => {
    await checkSession()
    if ($authenticated) {
      restoreLastRoute()
      await Promise.all([loadTree(), loadTags()])
    }
  })

  // When auth state changes to true, load data
  // chatAvailable espelha embed.key_configured: sem GEMINI_API_KEY o ícone do
  // chat aparece DESABILITADO com tooltip, nunca escondido — esconder impede a
  // descoberta de quem só precisa colar uma chave, e falhar em silêncio é o
  // modo de falha que ADR-004 D7 eliminou.
  let chatAvailable = $state(false)

  $effect(() => {
    if ($authenticated) {
      restoreLastRoute()
      Promise.all([loadTree(), loadTags()])
      apiGet('/api/admin/settings')
        .then(s => { chatAvailable = s?.['embed.key_configured'] === 'true' })
        .catch(() => { chatAvailable = false })
    }
  })

  const route = $derived(getRoute())

  const detailTitle = $derived(
    route.view === 'doc' ? ($activeDoc?.title || 'Documento') :
    route.view === 'graph' ? 'Grafo' :
    route.view === 'calendar' ? 'Calendário' :
    route.view === 'admin' ? 'Administração' :
    route.view === 'chat' ? 'Chat' : ''
  )
</script>

{#if $authenticated === null}
  <!-- Checking session -->
  <div style="display:flex;align-items:center;justify-content:center;height:100dvh;">
    <div class="spinner"></div>
  </div>

{:else if $authenticated === false}
  <LoginPage />

{:else if route.view === 'focus' && route.id}
  <div class="focus-layout">
    {#key route.id}
      <Editor docId={route.id} focusMode={true} />
    {/key}
  </div>

{:else}
  <div id="app">
    <!-- Top bar -->
    <header class="topbar" class:topbar-mobile-list={isMobile && route.view === 'home'}>
      {#if isMobile}
        {#if route.view === 'home'}
          <!-- MOBILE LISTA: duas linhas -->
          <div class="topbar-row topbar-row-icons">
            <a href="#/" class="app-logo">PKD</a>
            <span class="topbar-spacer"></span>
            {#if chatAvailable}
              <a href="#/chat" class="icon-btn" title="Chat com os documentos">💬</a>
            {:else}
              <span class="icon-btn icon-btn-disabled" title="Chat indisponível: GEMINI_API_KEY não configurada no servidor" aria-disabled="true">💬</span>
            {/if}
            <a href="#/graph" class="icon-btn" title="Grafo">🕸️</a>
            <a href="#/calendar" class="icon-btn" title="Calendário">📅</a>
            <a href="#/admin" class="icon-btn" title="Administração">⚙️</a>
            <button class="icon-btn" onclick={toggleTheme} title="Alternar tema" aria-label="Alternar tema">{theme === 'light' ? '🌙' : '☀️'}</button>
            <button class="icon-btn" onclick={logout} title="Sair" aria-label="Sair">⏻</button>
          </div>
          <div class="topbar-row topbar-row-search">
            <span class="search-icon" aria-hidden="true">🔍</span>
            <input class="topbar-search-mobile" type="search"
                   placeholder="Buscar documentos…" autocomplete="off"
                   value={$textFilter} oninput={onFilterInput}
                   aria-label="Buscar documentos" />
            {#if hasActiveFilters}
              <button class="topbar-reset-btn" onclick={resetFilters} title="Limpar filtros" aria-label="Limpar filtros">×</button>
            {/if}
          </div>
        {:else}
          <!-- MOBILE DETALHE: linha única com voltar -->
          <a href="#/" class="icon-btn" aria-label="Voltar" title="Voltar">←</a>
          <span class="topbar-title">{detailTitle}</span>
          {#if route.view === 'doc' && route.id}
            <button class="icon-btn" title="Compartilhar" onclick={() => openShare(Number(route.id))}>🔗</button>
          {/if}
        {/if}
      {:else}
        <!-- DESKTOP: topbar original -->
        <button class="icon-btn desktop-only" onclick={toggleSidebarCollapse}
                title={sidebarCollapsed ? 'Mostrar barra lateral' : 'Ocultar barra lateral'}
                aria-label={sidebarCollapsed ? 'Mostrar barra lateral' : 'Ocultar barra lateral'}>
          {sidebarCollapsed ? '▶' : '◀'}
        </button>

        <button class="icon-btn nav-btn" onclick={goBack} disabled={!canGoBack} title="Voltar (Alt+←)" aria-label="Voltar">←</button>
        <button class="icon-btn nav-btn" onclick={goForward} disabled={!canGoForward} title="Avançar (Alt+→)" aria-label="Avançar">→</button>

        <a href="#/" class="app-logo">PKD</a>

        <input
          class="topbar-search"
          type="search"
          placeholder="Filtrar…"
          autocomplete="off"
          value={$textFilter}
          oninput={onFilterInput}
          aria-label="Filtrar documentos"
        />
        {#if hasActiveFilters}
          <button class="topbar-reset-btn" onclick={resetFilters} title="Limpar todos os filtros" aria-label="Limpar todos os filtros">×</button>
        {/if}

        {#if chatAvailable}
          <a href="#/chat" class="icon-btn" title="Chat com os documentos">💬</a>
        {:else}
          <span class="icon-btn icon-btn-disabled" title="Chat indisponível: GEMINI_API_KEY não configurada no servidor" aria-disabled="true">💬</span>
        {/if}
        <a href="#/graph" class="icon-btn" title="Grafo">🕸️</a>
        <a href="#/calendar" class="icon-btn" title="Calendário">📅</a>
        <a href="#/admin" class="icon-btn" title="Administração">⚙️</a>

        {#if route.view === 'doc' && route.id}
          <button class="icon-btn" title="Compartilhar" onclick={() => openShare(Number(route.id))}>🔗</button>
        {/if}

        <button class="icon-btn" onclick={toggleTheme} title="Alternar tema" aria-label="Alternar tema">
          {theme === 'light' ? '🌙' : '☀️'}
        </button>

        <button class="icon-btn" onclick={logout} title="Sair" aria-label="Sair">⏻</button>

        {#if route.view === 'doc' && route.id}
          <button class="icon-btn desktop-only" onclick={toggleAssocCollapse}
                  title={assocCollapsed ? 'Mostrar painel de associações' : 'Ocultar painel de associações'}
                  aria-label={assocCollapsed ? 'Mostrar painel de associações' : 'Ocultar painel de associações'}>
            {assocCollapsed ? '◀' : '▶'}
          </button>
        {/if}
      {/if}
    </header>

    <!-- Main layout -->
    <div class="app-layout" class:mobile-list={isMobile && route.view === 'home'} class:mobile-detail={isMobile && route.view !== 'home'}>
      <!-- Sidebar with mobile overlay -->
      <aside class="sidebar"
             style={!isMobile && sidebarCollapsed ? 'width:0;border-right:none' : `width:${sidebarWidth}px`}
             aria-label="Navegação">
        <Sidebar onClearFilter={clearFilter} />
      </aside>
      <!-- Drag handle for desktop resize -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="resize-handle {resizing ? 'active' : ''} {!isMobile && sidebarCollapsed ? 'hidden' : ''}"
        onmousedown={onResizeStart}
        role="separator"
        aria-orientation="vertical"
        aria-label="Redimensionar painel lateral"
      ></div>

      <!-- Main content area -->
      <main class="content-area">
        {#if route.view === 'doc' && route.id}
          {#key route.id}
            <Editor docId={route.id} assocPortal={assocSidebarEl} />
          {/key}
        {:else if route.view === 'graph'}
          <GraphView />
        {:else if route.view === 'calendar'}
          <Calendar />
        {:else if route.view === 'admin'}
          <Admin />
        {:else if route.view === 'chat'}
          <Chat />
        {:else}
          <!-- Home / empty state -->
          <div class="empty-state" style="flex:1">
            <span class="emoji">📚</span>
            <p>Selecione um documento na barra lateral<br>ou crie um novo para começar.</p>
          </div>
        {/if}
      </main>

      <!-- Right associations panel (desktop only) -->
      {#if route.view === 'doc' && route.id && !isMobile}
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div
          class="resize-handle-assoc {resizingAssoc ? 'active' : ''} {assocCollapsed ? 'hidden' : ''}"
          onmousedown={onAssocResizeStart}
          role="separator"
          aria-orientation="vertical"
          aria-label="Redimensionar painel de associações"
        ></div>
        <aside class="assoc-sidebar" bind:this={assocSidebarEl}
               style={assocCollapsed ? 'width:0;border-left:none' : `width:${assocPanelWidth}px`}
               aria-label="Associações"></aside>
      {/if}
    </div>
  </div>

  <!-- Share dialog -->
  {#if shareDocId}
    <ShareDialog docId={shareDocId} onClose={closeShare} />
  {/if}
{/if}

<style>
  .nav-btn { font-size: .95rem; letter-spacing: 0; }

  .topbar-search {
    flex: 1;
    min-width: 0;
    max-width: 200px;
    padding: .2rem .45rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: .8rem;
    background: var(--bg);
    color: var(--text);
  }
  .topbar-search:focus { outline: none; border-color: var(--accent); }

  .topbar-reset-btn {
    flex-shrink: 0;
    width: 26px;
    height: 26px;
    border-radius: var(--radius);
    font-size: .95rem;
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-hover);
    border: 1px solid var(--border);
    margin-left: -.25rem;
  }
  .topbar-reset-btn:hover { background: var(--bg-active); color: var(--accent); border-color: var(--accent); }

  .resize-handle-assoc {
    width: 4px;
    cursor: col-resize;
    background: transparent;
    flex-shrink: 0;
    transition: background .15s;
    z-index: 10;
  }
  .resize-handle-assoc:hover,
  .resize-handle-assoc.active { background: var(--accent); }
  .resize-handle-assoc.hidden { display: none; }

  .assoc-sidebar {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--bg-panel);
    border-left: 1px solid var(--border);
    transition: width .2s ease;
  }

  .desktop-only { display: none; }

  @media (min-width: 641px) {
    .desktop-only { display: inline-flex; }
  }

  .focus-layout {
    width: 100dvw;
    height: 100dvh;
    display: flex;
    overflow: hidden;
    background: var(--bg);
  }

  .app-logo {
    font-size: .9rem;
    font-weight: 700;
    color: var(--accent);
    text-decoration: none;
    letter-spacing: -.5px;
    flex-shrink: 0;
  }

  .topbar-row {
    display: flex;
    align-items: center;
    gap: .25rem;
    padding: .3rem .5rem;
  }
  .topbar-row-icons { border-bottom: 1px solid var(--border); min-height: 44px; }
  .topbar-row-search { gap: .4rem; }
  .topbar-spacer { flex: 1; }
  .search-icon { font-size: 1rem; flex-shrink: 0; opacity: .5; }

  .topbar-search-mobile {
    flex: 1;
    min-width: 0;
    padding: .55rem .6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: .95rem;
    background: var(--bg);
    color: var(--text);
  }
  .topbar-search-mobile:focus { outline: none; border-color: var(--accent); }

  .topbar-title {
    flex: 1;
    font-weight: 600;
    font-size: .9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }
</style>
