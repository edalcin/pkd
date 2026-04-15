# PKD Security Reference

This document describes every security control in PKD, why it exists, and what it does not protect against.

---

## Threat model

PKD is a **single-user, self-hosted** tool. The threat model is:

- **Threat**: an unauthorized person tries to read or modify your notes via the network.
- **Not a threat**: physical access to the host machine, a compromised host OS, or a malicious Docker image.
- **Partial threat**: a guest on the same network who can observe traffic (mitigated by running behind HTTPS).

---

## Authentication and session management

| Control | Mechanism |
|---|---|
| Master password | Supplied at container start via `PKD_PASSWORD` env var. Never stored in the database. Compared at login using `crypto/subtle.ConstantTimeCompare` over SHA-256 digests of both sides — prevents timing attacks even for different-length inputs. |
| Session token | 32 cryptographically random bytes (base64url encoded). Stored in an in-memory map. Expires after `PKD_SESSION_IDLE_MINUTES` of inactivity (default: 60 min). Lost on container restart (user must log in again). |
| Session cookie | `HttpOnly; SameSite=Strict; Path=/`. Secure flag is intentionally absent because PKD often runs on a LAN without TLS — set `Secure` manually if you control the Go source build. |
| Failed-login lockout | 5 consecutive failures from the same source IP → 30-minute lockout. Counter resets on a successful login. `Retry-After` header tells the client the wait time. |
| IP detection | By default `RemoteAddr`. Set `PKD_TRUST_PROXY_HEADERS=1` when behind a trusted reverse proxy to use `X-Forwarded-For` instead. **Do not set this flag without a proxy — it allows IP spoofing.** |

---

## Transport

PKD does not manage TLS certificates. Terminate TLS at a reverse proxy (Caddy, Traefik, UNRAID SWAG). Without TLS, session cookies transit in cleartext on the LAN.

---

## CSRF protection

Double-submit cookie pattern:
- On every `GET`, if `pkd_csrf` cookie is absent, the server sets one with a 32-byte random value.
- On every mutating request (POST/PUT/DELETE/PATCH), the `X-CSRF-Token` header must match the `pkd_csrf` cookie. Mismatch → 403.
- The CSRF cookie is **not** HttpOnly so JavaScript can read it and put it in the header.

---

## Content Security Policy

Two distinct CSPs are in use:

| Scope | Policy summary |
|---|---|
| Authenticated SPA (`/`, `/api/*`) | `script-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'` |
| Public share view (`/public/{token}`) | `script-src 'none'; img-src 'self' data:; style-src 'self'; frame-ancestors 'none'` |

The public share view intentionally sets `script-src 'none'` — no JavaScript runs on the shared page. This means even if a malicious payload somehow bypassed HTML sanitization, it cannot execute.

Additional hardening headers on every response: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Strict-Transport-Security: max-age=31536000; includeSubDomains`, `Permissions-Policy: interest-cohort=()`.

---

## HTML sanitization

Rich text entered in CKEditor passes through `bluemonday` before storage:

- **EditorPolicy**: allows formatting tags, images (with resize width style), tables, links (http/https/mailto only), code blocks. Strips all event handlers, `<script>`, `<style>`, `javascript:` URIs, and SVG `<foreignObject>`.
- **PublicSharePolicy**: same as EditorPolicy but also strips inline styles and data attributes.

Plain-text is derived from the sanitized HTML for FTS5 indexing.

---

## Share link tokens

- 32 random bytes generated via `crypto/rand`.
- Encoded as base64url (43 characters).
- SHA-256 hash stored in `share_links.token_hash` — the plaintext is **never** persisted.
- `LookupByToken` iterates active rows comparing hashes with `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.
- Revocation sets `revoked_at`; the public endpoint always returns 404 for revoked or missing tokens (never 401 or 410) to avoid leaking existence.

---

## Attachment path traversal defense

`security.SafeAttachmentPath(base, stored)`:
1. Rejects empty string.
2. Rejects null bytes.
3. Rejects Unix-style absolute paths (`/…`) and Windows absolute paths.
4. Rejects `..` components.
5. Calls `filepath.Clean(filepath.Join(base, stored))` and verifies the result is still under `base`.

---

## Database

- SQLite with `PRAGMA foreign_keys = ON` (enforces referential integrity).
- WAL journal mode for concurrent reads during backup.
- All queries use parameterized statements — no string interpolation in SQL.
- Database file lives outside the container on a host-mounted volume.

---

## What PKD does not protect against

| Threat | Not protected |
|---|---|
| Host OS compromise | If an attacker has root on the host, they can read the SQLite file directly. |
| Physical access | Same as above. |
| Brute force of very long sessions | Session IDs are 32 random bytes — 256-bit entropy. Not feasible to brute-force, but the in-memory store has no rate limit on session lookup (only on login). |
| Timing attacks on session IDs | Session lookup does a map key lookup, which is not constant-time. This is acceptable because session IDs are high-entropy and the attacker would need billions of guesses to find a valid one. |
| Attachment MIME confusion | The MIME type stored in the database is whatever the uploader sent. Downloads set `Content-Disposition: attachment` to prevent in-browser execution, but a malicious uploader could store a wrong MIME type. |
