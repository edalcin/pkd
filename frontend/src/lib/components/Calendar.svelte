<script>
  import { onMount } from 'svelte'
  import { apiGet } from '../api.js'

  let today = new Date()
  let year = $state(today.getFullYear())
  let month = $state(today.getMonth() + 1)
  let calData = $state([]) // [{day, documents: [{id, title, icon}]}]
  let selectedDay = $state(null)
  let loading = $state(true)

  const WEEKDAYS = ['Dom', 'Seg', 'Ter', 'Qua', 'Qui', 'Sex', 'Sáb']
  const MONTHS = ['', 'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho',
    'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro']

  onMount(() => loadMonth())

  async function loadMonth() {
    loading = true
    selectedDay = null
    try {
      calData = await apiGet(`/api/calendar/${year}/${month}`)
    } finally {
      loading = false
    }
  }

  function prevMonth() {
    if (month === 1) { month = 12; year-- } else { month-- }
    loadMonth()
  }

  function nextMonth() {
    if (month === 12) { month = 1; year++ } else { month++ }
    loadMonth()
  }

  // Build grid
  function buildGrid() {
    const firstDay = new Date(year, month - 1, 1).getDay()
    const daysInMonth = new Date(year, month, 0).getDate()
    const grid = []
    // Leading empty cells
    for (let i = 0; i < firstDay; i++) grid.push(null)
    for (let d = 1; d <= daysInMonth; d++) grid.push(d)
    // Trailing empty cells for full weeks
    while (grid.length % 7 !== 0) grid.push(null)
    return grid
  }

  function docsForDay(day) {
    return calData.find(e => e.day === day)?.documents || []
  }

  function selectDay(day) {
    selectedDay = selectedDay === day ? null : day
  }
</script>

<div class="cal-wrap">
  <!-- Header -->
  <div class="cal-header">
    <button class="icon-btn" onclick={prevMonth} aria-label="Mês anterior">‹</button>
    <h2 class="cal-title">{MONTHS[month]} {year}</h2>
    <button class="icon-btn" onclick={nextMonth} aria-label="Próximo mês">›</button>
  </div>

  {#if loading}
    <div class="empty-state"><div class="spinner"></div></div>
  {:else}
    <!-- Weekday headers -->
    <div class="calendar-grid weekday-header">
      {#each WEEKDAYS as wd}
        <div class="weekday-label">{wd}</div>
      {/each}
    </div>

    <!-- Day cells -->
    <div class="calendar-grid">
      {#each buildGrid() as day}
        {#if day === null}
          <div class="cal-day empty"></div>
        {:else}
          {@const docs = docsForDay(day)}
          <button
            class="cal-day {docs.length > 0 ? 'has-docs' : ''} {selectedDay === day ? 'selected' : ''}"
            onclick={() => selectDay(day)}
            aria-label="{day}/{month}/{year}, {docs.length} documentos"
          >
            <span>{day}</span>
            {#if docs.length > 0}<span class="dot"></span>{/if}
          </button>
        {/if}
      {/each}
    </div>

    <!-- Selected day documents -->
    {#if selectedDay !== null}
      {@const docs = docsForDay(selectedDay)}
      <div class="day-docs">
        <h3>{selectedDay} de {MONTHS[month]}</h3>
        {#if docs.length === 0}
          <p class="muted">Nenhum documento criado neste dia.</p>
        {:else}
          <div class="doc-scroll">
            {#each docs as doc}
              <a class="day-doc-link" href="#{`/doc/${doc.id}`}">
                <i class="bx {doc.icon || 'bx-file-blank'}"></i> {doc.title}
              </a>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .cal-wrap {
    padding: 1.5rem 2rem;
    max-width: 600px;
    margin: 0 auto;
    width: 100%;
  }

  .cal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  .cal-title { font-size: 1.125rem; font-weight: 600; }

  .weekday-header { margin-bottom: 2px; }
  .weekday-label {
    text-align: center;
    font-size: .75rem;
    font-weight: 600;
    color: var(--text-muted);
    padding: .25rem 0;
  }

  .cal-day {
    aspect-ratio: 1;
    min-height: 40px;
    border-radius: var(--radius);
    background: none;
    border: none;
    cursor: pointer;
    font-size: .9rem;
    color: var(--text);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
  }
  .cal-day:hover { background: var(--bg-hover); }
  .cal-day.has-docs { color: var(--accent); font-weight: 600; }
  .cal-day.selected { background: var(--bg-active); }
  .cal-day.empty { cursor: default; }

  .day-docs {
    margin-top: 1.25rem;
    padding: .75rem;
    background: var(--bg-panel);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
  }

  .day-docs h3 { font-size: .9rem; margin-bottom: .5rem; }

  .doc-scroll {
    display: flex;
    flex-direction: row;
    gap: .5rem;
    overflow-x: auto;
    padding-bottom: .25rem;
    scrollbar-width: thin;
  }

  .day-doc-link {
    display: flex;
    align-items: center;
    gap: .375rem;
    padding: .3rem .6rem;
    font-size: .875rem;
    color: var(--text);
    cursor: pointer;
    flex-shrink: 0;
    white-space: nowrap;
    background: var(--bg-hover);
    border-radius: var(--radius);
  }
  .day-doc-link:hover { color: var(--accent); background: var(--bg-active); }

  .muted { color: var(--text-muted); font-size: .875rem; }
</style>
