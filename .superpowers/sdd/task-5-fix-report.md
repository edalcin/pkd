# Task 5 Fix Report — ShareDialog.svelte Race Condition

## Issues Fixed

### Issue 1: Race condition in `wasRecursive` capture
- **Root cause**: `wasRecursive = includeChildren` was assigned after the `await apiPost(...)` boundary. If the user toggled the checkbox during the network request, the badge would reflect the *new* checkbox state rather than the state actually sent to the API.
- **Fix**: Snapshot `includeChildren` into `const ic` before `loading = true` (and thus before any async boundary). Both `include_children: ic` in the POST body and `wasRecursive = ic` post-await now consistently reflect exactly what was sent.

### Issue 2: Checkbox enabled during loading
- **Root cause**: The checkbox had no `disabled` attribute, so users could toggle it while the request was in flight — making the race possible in practice.
- **Fix**: Added `disabled={loading}` to the checkbox `<input>`.

## Changes

File: `frontend/src/lib/components/ShareDialog.svelte`

```diff
- async function generateLink() {
-   loading = true
+ async function generateLink() {
+   const ic = includeChildren  // snapshot before any async boundary
+   loading = true
    try {
      const data = await apiPost(`/api/documents/${docId}/shares`, {
-       include_children: includeChildren,
+       include_children: ic,
      })
      shareUrl = data.url || `${window.location.origin}/public/${data.token}`
      shareId = data.revoke_id
-     wasRecursive = includeChildren
+     wasRecursive = ic  // reflects exactly what was sent to the API
    } finally {
      loading = false
    }
  }

- <input type="checkbox" bind:checked={includeChildren} />
+ <input type="checkbox" bind:checked={includeChildren} disabled={loading} />
```

## Verification

- Build: `npm run build` — ✓ built in 7.10s (no errors; pre-existing a11y warnings unchanged)
- Commit: `df59d0a fix(share): snapshot includeChildren before async in generateLink`
