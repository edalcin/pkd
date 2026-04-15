/**
 * admin.js — Administration panel
 * Backup, restore, cleanup, tag rename, trash management
 */
import { apiFetch } from './app.js';

export async function renderAdmin(container) {
  container.innerHTML = `
    <div style="padding:1.5rem;max-width:640px">
      <h2 style="margin-bottom:1.5rem;font-size:1.25rem">Administration</h2>

      <section style="margin-bottom:2rem">
        <h3 style="margin-bottom:.75rem">Backup & Restore</h3>
        <div style="display:flex;gap:.75rem;flex-wrap:wrap">
          <button id="backup-btn" class="icon-btn" style="padding:.5rem 1rem;border:1px solid var(--border);border-radius:6px">Download backup</button>
          <label style="display:flex;align-items:center;gap:.5rem;cursor:pointer;padding:.5rem 1rem;border:1px solid var(--border);border-radius:6px">
            <input type="file" id="restore-file" accept=".sqlite" style="display:none">
            Choose backup file…
          </label>
        </div>
        <div id="restore-confirm" class="hidden" style="margin-top:.75rem;display:flex;gap:.5rem;align-items:center">
          <input type="text" id="restore-confirm-input" placeholder="Type REPLACE to confirm" style="padding:.375rem .625rem;border:1px solid var(--border);border-radius:6px;font-size:.875rem;background:var(--bg);color:var(--text)">
          <button id="restore-btn" style="padding:.375rem .75rem;background:var(--danger);color:#fff;border:none;border-radius:6px;cursor:pointer">Restore</button>
        </div>
      </section>

      <section style="margin-bottom:2rem">
        <h3 style="margin-bottom:.75rem">Maintenance</h3>
        <button id="cleanup-btn" class="icon-btn" style="padding:.5rem 1rem;border:1px solid var(--border);border-radius:6px">Run cleanup</button>
        <span id="cleanup-result" style="margin-left:.75rem;font-size:.875rem;color:var(--text-muted)"></span>
      </section>

      <section style="margin-bottom:2rem">
        <h3 style="margin-bottom:.75rem">Rename / Merge Tag</h3>
        <div style="display:flex;gap:.5rem;flex-wrap:wrap">
          <input id="tag-old" type="text" placeholder="Existing tag" style="padding:.375rem .625rem;border:1px solid var(--border);border-radius:6px;font-size:.875rem;background:var(--bg);color:var(--text)">
          <span style="line-height:2rem">→</span>
          <input id="tag-new" type="text" placeholder="New name" style="padding:.375rem .625rem;border:1px solid var(--border);border-radius:6px;font-size:.875rem;background:var(--bg);color:var(--text)">
          <button id="tag-rename-btn" style="padding:.375rem .75rem;background:var(--accent);color:var(--accent-text);border:none;border-radius:6px;cursor:pointer">Rename</button>
        </div>
      </section>

      <section>
        <h3 style="margin-bottom:.75rem">Trash</h3>
        <button id="empty-trash-btn" style="padding:.375rem .75rem;background:var(--danger);color:#fff;border:none;border-radius:6px;cursor:pointer;margin-bottom:.75rem">Empty Trash</button>
        <div id="trash-list"></div>
      </section>
    </div>`;

  // Backup
  document.getElementById('backup-btn').addEventListener('click', async () => {
    const res = await apiFetch('/api/admin/backup', { method: 'POST' });
    if (!res.ok) { alert('Backup failed'); return; }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'pkd-backup.sqlite';
    a.click();
  });

  // Restore file selection
  document.getElementById('restore-file').addEventListener('change', e => {
    if (e.target.files.length) {
      document.getElementById('restore-confirm').classList.remove('hidden');
    }
  });

  document.getElementById('restore-btn')?.addEventListener('click', async () => {
    if (document.getElementById('restore-confirm-input').value !== 'REPLACE') {
      alert('Type REPLACE in the confirmation field'); return;
    }
    const file = document.getElementById('restore-file').files[0];
    if (!file) return;
    const fd = new FormData();
    fd.append('file', file);
    fd.append('confirm', 'REPLACE');
    const res = await apiFetch('/api/admin/restore', { method: 'POST', body: fd });
    if (res.ok) { alert('Restore successful. Please log in again.'); window.location.replace('/login'); }
    else { alert('Restore failed: ' + await res.text()); }
  });

  // Cleanup
  document.getElementById('cleanup-btn').addEventListener('click', async () => {
    const res = await apiFetch('/api/admin/cleanup', { method: 'POST' });
    if (res.ok) {
      const data = await res.json();
      document.getElementById('cleanup-result').textContent = `Removed ${data.orphans_removed} orphaned files.`;
    }
  });

  // Tag rename
  document.getElementById('tag-rename-btn').addEventListener('click', async () => {
    const old = document.getElementById('tag-old').value.trim();
    const nw = document.getElementById('tag-new').value.trim();
    if (!old || !nw) { alert('Fill both fields'); return; }
    const res = await apiFetch('/api/admin/tags/rename', { method: 'PUT', body: JSON.stringify({ old, new: nw }) });
    if (res.ok) { alert('Tag renamed'); }
    else { alert('Error: ' + await res.text()); }
  });

  // Empty trash
  document.getElementById('empty-trash-btn').addEventListener('click', async () => {
    if (!confirm('Permanently delete ALL trashed documents?')) return;
    await apiFetch('/api/admin/trash/empty', { method: 'POST' });
    await loadTrash();
  });

  await loadTrash();
}

async function loadTrash() {
  const res = await apiFetch('/api/admin/trash');
  if (!res.ok) return;
  const docs = await res.json() || [];
  const list = document.getElementById('trash-list');
  if (!list) return;
  list.innerHTML = '';
  if (!docs.length) { list.textContent = 'Trash is empty.'; return; }
  for (const doc of docs) {
    const row = document.createElement('div');
    row.style.cssText = 'display:flex;align-items:center;gap:.5rem;padding:.375rem 0;border-bottom:1px solid var(--border);font-size:.875rem';
    row.innerHTML = `<span style="flex:1">${doc.title}</span>`;
    const restore = document.createElement('button');
    restore.textContent = 'Restore';
    restore.style.cssText = 'padding:.2rem .5rem;border:1px solid var(--border);border-radius:4px;cursor:pointer;font-size:.8rem;background:var(--bg);color:var(--text)';
    restore.addEventListener('click', async () => {
      await apiFetch(`/api/documents/${doc.id}/restore`, { method: 'POST' });
      await loadTrash();
    });
    const del = document.createElement('button');
    del.textContent = 'Delete';
    del.style.cssText = 'padding:.2rem .5rem;border:none;border-radius:4px;cursor:pointer;font-size:.8rem;background:var(--danger);color:#fff';
    del.addEventListener('click', async () => {
      if (!confirm(`Permanently delete "${doc.title}"?`)) return;
      await apiFetch(`/api/admin/trash/${doc.id}`, { method: 'DELETE' });
      await loadTrash();
    });
    row.append(restore, del);
    list.appendChild(row);
  }
}
