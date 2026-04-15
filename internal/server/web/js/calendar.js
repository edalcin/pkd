/**
 * calendar.js — Chronological calendar browsing.
 * Shows a month grid with document counts per day.
 */
import { apiFetch } from './app.js';

export async function renderCalendar(container, onSelect, initialYear, initialMonth) {
  let year = initialYear ?? new Date().getFullYear();
  let month = initialMonth ?? (new Date().getMonth() + 1);

  async function load() {
    const res = await apiFetch(`/api/calendar/${year}/${month}`);
    if (!res.ok) return;
    const days = await res.json();
    draw(days);
  }

  function draw(days) {
    container.innerHTML = '';

    const header = document.createElement('div');
    header.style.cssText = 'display:flex;align-items:center;justify-content:space-between;padding:.75rem;border-bottom:1px solid var(--border)';
    const prev = document.createElement('button');
    prev.textContent = '‹';
    prev.className = 'icon-btn';
    prev.onclick = () => { month--; if (month < 1) { month = 12; year--; } load(); };
    const title = document.createElement('strong');
    title.textContent = new Date(year, month - 1).toLocaleString('default', { month: 'long', year: 'numeric' });
    const next = document.createElement('button');
    next.textContent = '›';
    next.className = 'icon-btn';
    next.onclick = () => { month++; if (month > 12) { month = 1; year++; } load(); };
    header.append(prev, title, next);
    container.appendChild(header);

    // Build day map
    const dayMap = {};
    (days || []).forEach(d => { dayMap[d.day] = d.documents; });

    // Calendar grid
    const grid = document.createElement('div');
    grid.style.cssText = 'display:grid;grid-template-columns:repeat(7,1fr);gap:2px;padding:.5rem';

    ['Su','Mo','Tu','We','Th','Fr','Sa'].forEach(d => {
      const cell = document.createElement('div');
      cell.textContent = d;
      cell.style.cssText = 'text-align:center;font-size:.7rem;color:var(--text-muted);padding:.25rem 0';
      grid.appendChild(cell);
    });

    const firstDay = new Date(year, month - 1, 1).getDay();
    const daysInMonth = new Date(year, month, 0).getDate();

    for (let i = 0; i < firstDay; i++) {
      grid.appendChild(document.createElement('div'));
    }

    for (let d = 1; d <= daysInMonth; d++) {
      const cell = document.createElement('div');
      const docs = dayMap[d] || [];
      cell.style.cssText = 'text-align:center;padding:.375rem .25rem;border-radius:4px;font-size:.8rem;min-height:2rem;position:relative;cursor:' + (docs.length ? 'pointer' : 'default');
      cell.textContent = d;
      if (docs.length) {
        cell.style.background = 'var(--bg-active)';
        cell.style.color = 'var(--accent)';
        cell.style.fontWeight = '600';
        cell.title = docs.map(doc => doc.title).join(', ');
        cell.addEventListener('click', () => {
          if (docs.length === 1) { onSelect(docs[0].id); return; }
          // Show mini list
          showDayPopover(cell, docs, onSelect);
        });
      }
      grid.appendChild(cell);
    }
    container.appendChild(grid);
  }

  function showDayPopover(anchor, docs, onSelect) {
    document.getElementById('day-popover')?.remove();
    const pop = document.createElement('div');
    pop.id = 'day-popover';
    pop.style.cssText = 'position:fixed;background:var(--bg-card);border:1px solid var(--border);border-radius:6px;padding:.5rem 0;z-index:300;min-width:180px;box-shadow:var(--shadow)';
    const rect = anchor.getBoundingClientRect();
    pop.style.top = rect.bottom + 4 + 'px';
    pop.style.left = rect.left + 'px';
    for (const doc of docs) {
      const item = document.createElement('div');
      item.textContent = doc.title;
      item.style.cssText = 'padding:.375rem .75rem;cursor:pointer;font-size:.875rem';
      item.addEventListener('click', () => { pop.remove(); onSelect(doc.id); });
      pop.appendChild(item);
    }
    document.body.appendChild(pop);
    setTimeout(() => document.addEventListener('click', () => pop.remove(), { once: true }), 0);
  }

  await load();
}
