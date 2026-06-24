# Task 3 Report: Handlers — criação e página pública

## Status: DONE

## Changes Made

### File: `internal/server/handlers_share.go`

**Step 1 — `handleCreateShare()` updated:**
- Added `"encoding/json"` to imports.
- Defined local `createShareRequest` struct with `IncludeChildren *bool` (`json:"include_children"`).
- Decodes JSON body only when `r.ContentLength > 0`; invalid JSON returns 400.
- Defaults `includeChildren = true` when `req.IncludeChildren` is nil (backward-compatible).
- Calls `s.shares.Create(docID, includeChildren)` — fixes the broken callsite from Task 2.
- `collectDescendantIDs` + `CreateAuto` loop gated behind `if includeChildren`.

**Step 2 — `handlePublicShare()` updated:**
- Replaced the unconditional children-fetch block with one guarded by `if shareLink.IncludeChildren { ... }`.
- `var childData []shareChildData` declared inside the `if` block (always defined as nil when flag is false).
- Uses `s.docs.ListChildren(doc.ID)` (not `ListByDocument`) as specified.
- `shareLink.IncludeChildren` populated by `LookupByToken()` (Task 2).

## Build

`go build ./...` — clean, no errors.

## Commit

- `dd3f99a` feat(share): handleCreateShare reads include_children; public page respects flag
