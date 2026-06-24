# Task 4 Fix Report — share_test.go

## Commit
`ce8172a fix(share): guard http.Get error in TestShareDefaultIsRecursive`

## Changes Made

### Issue 1 — `TestShareDefaultIsRecursive` (Important)

**Problem:** `pubResp, _ := http.Get(shareURL)` discarded the error, and there was no guard on an empty `shareURL`. If the GET failed, `pubResp` would be nil and the `defer pubResp.Body.Close()` would panic.

**Fix applied (lines 135–150):**
- Added `if shareURL == "" { t.Fatal(...) }` guard immediately after decoding `shareData`.
- Replaced `pubResp, _ := http.Get(shareURL)` with proper error capture and `t.Fatalf` on failure.
- Added HTTP 200 status code assertion matching the pattern used in `TestShareRecursive` and `TestShareNonRecursive`.

### Issue 2 — `TestShareRecursive` (Minor)

**Problem:** `child` variable was decoded and `childID` was computed but both were unused (only suppressed with `_ = childID`), introducing unnecessary variables.

**Fix applied (lines 23–26):**
- Collapsed `child := decodeDoc(...)` + `childID := int64(...)` + `_ = childID` into a single `_ = decodeDoc(...)`.

## Test Output

```
=== RUN   TestShareRecursive
--- PASS: TestShareRecursive (0.08s)
=== RUN   TestShareNonRecursive
--- PASS: TestShareNonRecursive (0.00s)
=== RUN   TestShareDefaultIsRecursive
--- PASS: TestShareDefaultIsRecursive (0.00s)
PASS
ok  github.com/edalcin/pkd/tests/integration  0.192s
```

3/3 PASS, no panics.
