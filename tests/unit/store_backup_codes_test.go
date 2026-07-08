package unit_test

import (
	"strings"
	"testing"

	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// openBackupCodeTestDB opens a fresh in-memory SQLite DB dedicated to this
// file's tests (unique DSN so it never shares state with other tests/unit
// files that also use store.Open's in-memory URI).
func openBackupCodeTestDB(t *testing.T) *store.BackupCodeStore {
	t.Helper()
	db, err := store.Open("file:store_backup_codes_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewBackupCodeStore(db)
}

// TestBackupCodeStore_ReplaceConsumeCount exercises the full lifecycle of
// backup-code recovery codes at the store layer: an empty store reports zero
// remaining, Replace seeds a fresh set, Consume is single-use and
// normalization-tolerant (case/dash-insensitive), unknown/empty input never
// consumes anything, and regenerating via Replace invalidates every
// previously issued code even though it was never consumed.
func TestBackupCodeStore_ReplaceConsumeCount(t *testing.T) {
	codes := openBackupCodeTestDB(t)

	if n, err := codes.Count(); err != nil || n != 0 {
		t.Fatalf("Count() on empty store = (%d, %v), want (0, nil)", n, err)
	}

	c1, c2, c3 := security.NewBackupCode(), security.NewBackupCode(), security.NewBackupCode()
	if err := codes.Replace([]string{c1, c2, c3}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if n, err := codes.Count(); err != nil || n != 3 {
		t.Fatalf("Count() after Replace = (%d, %v), want (3, nil)", n, err)
	}

	if used, err := codes.Consume(c1); err != nil || !used {
		t.Fatalf("Consume(c1) = (%v, %v), want (true, nil)", used, err)
	}
	if n, err := codes.Count(); err != nil || n != 2 {
		t.Fatalf("Count() after consuming c1 = (%d, %v), want (2, nil)", n, err)
	}

	// Single-use: consuming the same code again must fail now that its row
	// is gone.
	if used, err := codes.Consume(c1); err != nil || used {
		t.Fatalf("Consume(c1) again = (%v, %v), want (false, nil) — single-use", used, err)
	}

	// Normalization: uppercasing and stripping dashes/spaces must still
	// match the stored hash of the canonical code.
	mangled := strings.ToLower(strings.ReplaceAll(c2, "-", ""))
	if used, err := codes.Consume(mangled); err != nil || !used {
		t.Fatalf("Consume(mangled c2) = (%v, %v), want (true, nil) — case/dash-insensitive", used, err)
	}

	if used, err := codes.Consume("not-a-code"); err != nil || used {
		t.Fatalf(`Consume("not-a-code") = (%v, %v), want (false, nil)`, used, err)
	}
	if used, err := codes.Consume(""); err != nil || used {
		t.Fatalf(`Consume("") = (%v, %v), want (false, nil)`, used, err)
	}

	// Regenerating the whole set must invalidate c3, which was never
	// consumed but belonged to the old (now discarded) set.
	c4, c5 := security.NewBackupCode(), security.NewBackupCode()
	if err := codes.Replace([]string{c4, c5}); err != nil {
		t.Fatalf("Replace (regenerate): %v", err)
	}
	if used, err := codes.Consume(c3); err != nil || used {
		t.Fatalf("Consume(c3) after regenerate = (%v, %v), want (false, nil) — old set invalidated", used, err)
	}
	if n, err := codes.Count(); err != nil || n != 2 {
		t.Fatalf("Count() after regenerate = (%d, %v), want (2, nil)", n, err)
	}
}
