# PKD Operations Guide

Day-to-day maintenance, backup/restore, and upgrade procedures.

---

## Backup

PKD does **not** take automatic backups. You must initiate them manually.

### In-app backup (recommended)

1. Log in to PKD.
2. Navigate to **Administration** (top bar or sidebar).
3. Click **Download backup**. Your browser downloads `pkd-backup.sqlite`.

This uses SQLite `VACUUM INTO`, which produces a live-consistent, defragmented snapshot even while writes are in flight (WAL mode). The downloaded file is a complete, valid SQLite database.

**Store the file somewhere safe** — external drive, cloud storage, a second machine. PKD does not email or upload backups anywhere.

### Host-level backup (alternative)

You can also copy the database file directly from the host:

```bash
cp /mnt/user/appdata/pkd/db/pkd.sqlite /mnt/user/backups/pkd-$(date +%Y%m%d).sqlite
```

This works safely with WAL mode but is slightly less consistent than the in-app backup. Back up the attachments directory too if you care about attached files:

```bash
rsync -a /mnt/user/appdata/pkd/attachments/ /mnt/user/backups/pkd-attachments/
```

### Backup schedule suggestion

- Daily backup → keep for 7 days.
- Weekly backup → keep for 4 weeks.
- Monthly backup → keep indefinitely.

---

## Restore

### In-app restore

1. Log in.
2. **Administration → Choose backup file…** → select your `.sqlite` file.
3. Type `REPLACE` in the confirmation field.
4. Click **Restore**. The server swaps the database file in place and prompts you to log in again.

### Manual restore (host-level)

1. Stop the container: `docker stop pkd`.
2. Replace the database file: `cp /path/to/backup.sqlite /mnt/user/appdata/pkd/db/pkd.sqlite`.
3. Start the container: `docker start pkd`.

---

## Orphan cleanup

Over time, deleted documents may leave orphaned attachment files on disk (files no longer referenced by any document row). To remove them:

**Administration → Run cleanup**

This:
1. Walks the attachments directory.
2. Deletes any files with no matching row in the `attachments` table.
3. Runs `VACUUM` on the database to reclaim freed space.

Safe to run at any time. Reports how many files were removed.

---

## Hashtag maintenance

### Rename a tag

**Administration → Rename/Merge Tag** → fill in the existing tag name and the new name → **Rename**.

- If the new name does not exist yet: the tag is renamed in place. Every document that had the old tag now shows the new tag.
- If the new name already exists: the two tags are **merged**. Every document from the old tag is reassigned to the existing tag. The old tag row is deleted.

---

## Trash management

Deleted documents go to Trash and stay there **indefinitely** until you empty them.

- **View trash**: Administration → Trash section. Lists every trashed document with its original location.
- **Restore one**: click **Restore** next to the document. It reappears under its original parent (or at root if the parent was also deleted).
- **Delete permanently**: click **Delete** next to the document. Irreversible.
- **Empty all**: click **Empty Trash**. Permanently deletes every document in the trash. Irreversible.

---

## PWA cache invalidation

The service worker (`sw.js`) caches the app shell indefinitely. After a PKD upgrade, the browser may serve the old shell.

To force a cache refresh:

1. Open the browser's developer tools → Application → Service Workers.
2. Click **Unregister**.
3. Reload the page. The browser fetches the new `sw.js` and re-caches the app shell.

Alternatively, increment the cache name version in `sw.js` before building the image (e.g., `pkd-shell-v2`). The activate handler already purges old cache names on install.

---

## Upgrades

### Docker (docker run or compose)

```bash
docker pull ghcr.io/edalcin/pkd:latest
docker stop pkd && docker rm pkd
# Re-run the original docker run command (all data is on volumes)
```

Or with docker compose:

```bash
docker compose pull
docker compose up -d
```

### UNRAID

1. Docker tab → click the PKD container icon → **Force Update**.
2. UNRAID pulls the latest image and restarts the container.
3. Your data on `/mnt/user/appdata/pkd/` is untouched.

### Schema migrations

PKD runs its full schema DDL (`CREATE TABLE IF NOT EXISTS`) on every start. It is safe to upgrade — the schema is always applied idempotently. There are no versioned migration files to manage.

---

## Logs

- **Docker**: `docker logs pkd` — shows startup messages and any runtime errors.
- **UNRAID**: Docker tab → click the container icon → **Logs**.
- PKD logs to stdout/stderr only; it does not write to files.

---

## Monitoring

PKD exposes a health endpoint:

```
GET /healthz
```

Returns `200 {"status":"ok"}` when the database is reachable, `503` otherwise. Use this with your monitoring tool of choice (e.g., Uptime Kuma, Healthchecks.io).
