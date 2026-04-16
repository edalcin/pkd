<script>
  import { searchQuery, searchResults, searching, debouncedSearch, clearSearch } from '../stores/search.js'

  let inputEl = $state(null)
  let showDropdown = $state(false)

  function onInput(e) {
    debouncedSearch(e.target.value)
    showDropdown = true
  }

  function onFocus() {
    if ($searchResults.length) showDropdown = true
  }

  function onBlur() {
    // Delay to allow click on result
    setTimeout(() => { showDropdown = false }, 150)
  }

  function selectResult(id) {
    window.location.hash = `/doc/${id}`
    clearSearch()
    showDropdown = false
    if (inputEl) inputEl.value = ''
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && $searchQuery.trim()) {
      window.location.hash = `/search?q=${encodeURIComponent($searchQuery)}`
      showDropdown = false
    }
    if (e.key === 'Escape') {
      clearSearch()
      showDropdown = false
    }
  }
</script>

<div class="search-wrap">
  <input
    bind:this={inputEl}
    class="search-input"
    type="search"
    placeholder="Buscar…"
    autocomplete="off"
    oninput={onInput}
    onfocus={onFocus}
    onblur={onBlur}
    onkeydown={onKeyDown}
    aria-label="Busca universal"
  />

  {#if showDropdown && ($searchResults.length > 0 || $searching)}
    <div class="search-results" role="listbox">
      {#if $searching}
        <div class="search-result-item">
          <div class="spinner" style="width:16px;height:16px;margin:auto"></div>
        </div>
      {:else}
        {#each $searchResults as result}
          <div
            class="search-result-item"
            onclick={() => selectResult(result.id)}
            role="option"
            aria-selected="false"
            tabindex="0"
            onkeydown={e => e.key === 'Enter' && selectResult(result.id)}
          >
            <h4>{result.title}</h4>
            {#if result.snippet}
              <p>{@html result.snippet}</p>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .search-wrap {
    position: relative;
    flex: 1;
    min-width: 0;
  }
</style>
