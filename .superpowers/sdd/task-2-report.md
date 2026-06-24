# Task 2 Report: Store — Create() and LookupByToken()

## Status: DONE

## Changes

**File modified:** `internal/store/shares.go`

### Create()
- Signature changed from `Create(docID int64)` to `Create(docID int64, includeChildren bool)`
- Added `ic int` local var (0/1 encoding of `includeChildren`)
- INSERT now includes `include_children` column with value `ic`
- Returned `*model.ShareLink` now has `IncludeChildren` field populated

### LookupByToken()
- SELECT expanded to include `include_children` column
- Added `var ic int` scan target
- `rows.Scan` updated to scan `ic` between `tokenHash` and `createdStr`
- On match: `sl.IncludeChildren = ic == 1`

## Build Verification

```
go build github.com/edalcin/pkd/internal/store  → OK (no output)
go build github.com/edalcin/pkd/internal/model  → OK (no output)
```

`go build ./...` intentionally not run — `handlers_share.go` call site still uses old arity (fixed in Task 3).

## Commit

`44992d6 feat(share): update Create/LookupByToken to handle include_children`

## Notes

No concerns. The store package compiles cleanly in isolation.
