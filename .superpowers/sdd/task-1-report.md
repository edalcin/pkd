# Task 1 Report: Migration e Model

## Status: DONE

## Changes Made

### 1. `internal/store/migrate.go`
Added one entry at the end of the `colMigrations` slice (after line 100):
```go
{`ALTER TABLE share_links ADD COLUMN include_children INTEGER NOT NULL DEFAULT 1`, "alter share_links include_children"},
```
This follows the existing idempotent pattern — "duplicate column name" errors are silently ignored.

### 2. `internal/model/share.go`
Added `IncludeChildren bool` field to `ShareLink` struct:
```go
type ShareLink struct {
    ID              int64      `json:"id"`
    DocumentID      int64      `json:"document_id"`
    IncludeChildren bool       `json:"include_children"`
    TokenHash       []byte     `json:"-"`
    CreatedAt       time.Time  `json:"created_at"`
    RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}
```

## Verification
- `go build ./...` — clean, no errors.

## Commit
- `8762f69` feat(share): add include_children column and model field

## Concerns
None.
