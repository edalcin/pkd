<script>
  import { onMount } from 'svelte'
  import { authenticated, checkSession, logout } from './lib/stores/auth.js'
  import { loadTree } from './lib/stores/documents.js'
  import { loadTags } from './lib/stores/tags.js'
  import LoginPage from './lib/components/LoginPage.svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import Editor from './lib/components/Editor.svelte'
  import GraphView from './lib/components/GraphView.svelte'
  import Calendar from './lib/components/Calendar.svelte'
  import Admin from './lib/components/Admin.svelte'
  import Search from './lib/components/Search.svelte'
  import ShareDialog from './lib/components/ShareDialog.svelte'

  // ─── Routing ─────────────────────────────────────────────
  let hash = $state(window.location.hash.slice(1) || '/')
  window.addEventListener('hashchange', () => { hash = window.location.hash.slice(1) || '/' })

  function getRoute() {
    if (hash.startsWith('/doc/')) return { view: 'doc', id: hash.split('/')[2] }
    if (hash.startsWith('/search')) return { view: 'search' }
    if (hash === '/graph') return { view: 'graph' }
    if (hash === '/calendar') return { view: 'calendar' }
    if (hash === '/admin') return { view: 'admin' }
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

  // ─── Sidebar (mobile) ─────────────────────────────────────
  let sidebarOpen = $state(false)
  function toggleSidebar() { sidebarOpen = !sidebarOpen }
  function closeSidebar() { sidebarOpen = false }

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

  // ─── Share dialog ─────────────────────────────────────────
  let shareDocId = $state(null)
  function openShare(id) { shareDocId = id }
  function closeShare() { shareDocId = null }

  // ─── Bootstrap ───────────────────────────────────────────
  onMount(async () => {
    await checkSession()
    if ($authenticated) {
      await Promise.all([loadTree(), loadTags()])
    }
  })

  // When auth state changes to true, load data
  $effect(() => {
    if ($authenticated) {
      Promise.all([loadTree(), loadTags()])
    }
  })

  const route = $derived(getRoute())
</script>

{#if $authenticated === null}
  <!-- Checking session -->
  <div style="display:flex;align-items:center;justify-content:center;height:100dvh;">
    <div class="spinner"></div>
  </div>

{:else if $authenticated === false}
  <LoginPage />

{:else}
  <div id="app">
    <!-- Top bar -->
    <header class="topbar">
      <!-- Hamburger (mobile) -->
      <button class="icon-btn" onclick={toggleSidebar} aria-label="Menu" title="Menu">
        ☰
      </button>

      <!-- Logo / home link -->
      <a href="#/" class="app-logo" onclick={closeSidebar}>PKD</a>

      <!-- Search -->
      <Search />

      <!-- Nav icons -->
      <a href="#/graph" class="icon-btn" title="Grafo" onclick={closeSidebar}>🕸️</a>
      <a href="#/calendar" class="icon-btn" title="Calendário" onclick={closeSidebar}>📅</a>
      <a href="#/admin" class="icon-btn" title="Administração" onclick={closeSidebar}>⚙️</a>

      <!-- Share button (only in doc view) -->
      {#if route.view === 'doc' && route.id}
        <button class="icon-btn" title="Compartilhar" onclick={() => openShare(Number(route.id))}>🔗</button>
      {/if}

      <!-- Theme toggle -->
      <button class="icon-btn" onclick={toggleTheme} title="Alternar tema" aria-label="Alternar tema">
        {theme === 'light' ? '🌙' : '☀️'}
      </button>

      <!-- Logout -->
      <button class="icon-btn" onclick={logout} title="Sair" aria-label="Sair">⏻</button>
    </header>

    <!-- Main layout -->
    <div class="app-layout">
      <!-- Sidebar with mobile overlay -->
      <aside class="sidebar {sidebarOpen ? 'open' : ''}" style="width:{sidebarWidth}px" aria-label="Navegação">
        <Sidebar onNavigate={() => sidebarOpen = false} />
      </aside>
      <!-- Drag handle for desktop resize -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="resize-handle {resizing ? 'active' : ''}"
        onmousedown={onResizeStart}
        role="separator"
        aria-orientation="vertical"
        aria-label="Redimensionar painel lateral"
      ></div>

      <!-- Overlay for mobile -->
      {#if sidebarOpen}
        <div
          class="sidebar-overlay"
          onclick={closeSidebar}
          role="button"
          aria-label="Fechar menu"
          tabindex="0"
          onkeydown={e => e.key === 'Enter' && closeSidebar()}
        ></div>
      {/if}

      <!-- Main content area -->
      <main class="content-area">
        {#if route.view === 'doc' && route.id}
          {#key route.id}
            <Editor docId={route.id} />
          {/key}
        {:else if route.view === 'graph'}
          <GraphView />
        {:else if route.view === 'calendar'}
          <Calendar />
        {:else if route.view === 'admin'}
          <Admin />
        {:else}
          <!-- Home / empty state -->
          <div class="empty-state" style="flex:1">
            <span class="emoji">📚</span>
            <p>Selecione um documento na barra lateral<br>ou crie um novo para começar.</p>
          </div>
        {/if}
      </main>
    </div>
  </div>

  <!-- Share dialog -->
  {#if shareDocId}
    <ShareDialog docId={shareDocId} onClose={closeShare} />
  {/if}
{/if}

<style>
  .app-logo {
    font-size: .9rem;
    font-weight: 700;
    color: var(--accent);
    text-decoration: none;
    letter-spacing: -.5px;
    flex-shrink: 0;
  }

  .sidebar-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,.3);
    z-index: 40;
  }

  @media (min-width: 641px) {
    .sidebar-overlay { display: none; }
  }
</style>
