package unit_test

import (
	"regexp"
	"testing"

	"github.com/edalcin/pkd/internal/security"
)

var backupCodeRe = regexp.MustCompile(`^[ABCDEFGHJKMNPQRSTVWXYZ23456789]{5}-[ABCDEFGHJKMNPQRSTVWXYZ23456789]{5}$`)

// TestNewBackupCode_Format confirms every generated code matches the
// documented shape: two 5-char groups from the 30-symbol unambiguous
// alphabet (no I, L, O, U, 0, 1), joined by a dash.
func TestNewBackupCode_Format(t *testing.T) {
	for range 100 {
		code := security.NewBackupCode()
		if !backupCodeRe.MatchString(code) {
			t.Fatalf("NewBackupCode() = %q, does not match %s", code, backupCodeRe.String())
		}
	}
}

// TestNewBackupCode_NoDuplicatesAcrossManyCalls guards the entropy contract:
// at ~49 bits of randomness, 1000 draws colliding would indicate a broken
// RNG or biased/narrowed alphabet, not bad luck.
func TestNewBackupCode_NoDuplicatesAcrossManyCalls(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := range 1000 {
		code := security.NewBackupCode()
		if seen[code] {
			t.Fatalf("duplicate backup code generated: %q (call #%d)", code, i)
		}
		seen[code] = true
	}
}

// TestNormalizeBackupCode covers the case/separator-insensitive matching a
// user's typed-in code must survive, plus the empty/all-excluded edge cases.
func TestNormalizeBackupCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase with dash and space", "ab-cde fghjk", "ABCDEFGHJK"},
		{"strips punctuation and excluded ambiguous letters/digits", "A1-B0!L.O,U;I", "AB"},
		{"empty input", "", ""},
		{"garbage-only input outside alphabet", "il0u1o-.,!?", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := security.NormalizeBackupCode(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeBackupCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
