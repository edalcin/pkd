# UNRAID Installation Guide — PKD

This guide walks you through installing PKD on UNRAID using the graphical **Docker → Add Container** UI. You do not need a terminal for any step.

> All screenshots are referenced by file name from `docs/assets/` — commit them when you have the actual screenshots. The step-by-step text is complete and sufficient to install without screenshots.

---

## Prerequisites

- UNRAID 6.12 or later
- Docker service running (Community Applications is optional — PKD is a direct container pull)
- Internet access to pull `ghcr.io/edalcin/pkd:latest` (or pre-pull on an internet-connected machine and transfer the image)

---

## Step 1 — Prepare host paths

Open the UNRAID file manager (Files tab) and create two directories:

| Purpose | Suggested path |
|---|---|
| SQLite database | `/mnt/user/appdata/pkd/db` |
| File attachments | `/mnt/user/appdata/pkd/attachments` |

Both directories must be writable by the non-root user inside the container (UID/GID 65532, the `nonroot` distroless user). UNRAID usually creates directories with world-writable permissions by default.

---

## Step 2 — Open Docker → Add Container

Navigate to **Docker tab → Add Container**.

---

## Step 3 — Fill in container settings

| Field | Value |
|---|---|
| **Name** | `pkd` |
| **Repository** | `ghcr.io/edalcin/pkd:latest` |
| **Network Type** | `Bridge` |
| **WebUI** | `http://[IP]:[PORT:8080]` |
| **Icon URL** | *(optional)* `https://raw.githubusercontent.com/edalcin/pkd/main/docs/assets/pkd-icon.png` |

---

## Step 4 — Add Port mapping

Click **Add another Path, Port, Variable, Label or Device** → **Port**.

| Field | Value |
|---|---|
| Name | `WebUI` |
| Container Port | `8080` |
| Host Port | `8080` *(change if 8080 is occupied)* |
| Connection Type | `TCP` |

---

## Step 5 — Add Path (database volume)

Click **Add another Path, Port, Variable, Label or Device** → **Path**.

| Field | Value |
|---|---|
| Name | `DB` |
| Container Path | `/data/db` |
| Host Path | `/mnt/user/appdata/pkd/db` |
| Access Mode | `Read/Write` |

---

## Step 6 — Add Path (attachments volume)

Click **Add another Path, Port, Variable, Label or Device** → **Path**.

| Field | Value |
|---|---|
| Name | `Attachments` |
| Container Path | `/data/attachments` |
| Host Path | `/mnt/user/appdata/pkd/attachments` |
| Access Mode | `Read/Write` |

---

## Step 7 — Add Variable: Master Password

Click **Add another Path, Port, Variable, Label or Device** → **Variable**.

| Field | Value |
|---|---|
| Name | `Master Password` |
| Key | `PKD_PASSWORD` |
| Value | *(generate with `openssl rand -base64 24`)* |
| Type | `Password` *(UNRAID masks the field)* |

**Do not reuse an existing password.** This is the only credential that protects your entire knowledge base.

---

## Step 8 — Add Variable: DB path

| Key | `PKD_DB_PATH` |
|---|---|
| Value | `/data/db/pkd.sqlite` |

---

## Step 9 — Add Variable: Attachments path

| Key | `PKD_ATTACHMENTS_PATH` |
|---|---|
| Value | `/data/attachments` |

---

## Step 10 — Apply

Click **Apply**. UNRAID pulls the image, creates the container, and starts it. The container log will show:

```
listening on :8080
schema ready at /data/db/pkd.sqlite
```

Click the **WebUI** link. Enter the master password from Step 7. You're in.

---

## Optional: Put PKD behind HTTPS

Share links generate public URLs. Those URLs should be HTTPS. Install the **SWAG** (Secure Web Application Gateway) plugin from UNRAID Community Applications, then add a subdomain proxy config that forwards to the `pkd` container on port `8080`.

When behind a reverse proxy, also set:

| Key | `PKD_TRUST_PROXY_HEADERS` |
|---|---|
| Value | `1` |

**Only set this when a trusted reverse proxy is in front.** Setting it on a directly-exposed container allows anyone to spoof their IP and bypass the authentication lockout.

---

## Upgrading

1. In the Docker tab, click the PKD container icon → **Force Update**. UNRAID will pull the new image and restart the container.
2. Your data is on the host volume and is never affected by image updates.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Container exits immediately | Required env var missing | Check `PKD_PASSWORD`, `PKD_DB_PATH`, `PKD_ATTACHMENTS_PATH` are all set |
| "permission denied" in logs | Host directory not writable by UID 65532 | `chmod 777 /mnt/user/appdata/pkd/db /mnt/user/appdata/pkd/attachments` |
| Login page loads but login fails | Password mismatch | Re-check the value in the PKD_PASSWORD variable; no leading/trailing spaces |
| 502 from SWAG | Wrong container port in SWAG config | Ensure SWAG proxies to `pkd:8080`, not `localhost:8080` |
