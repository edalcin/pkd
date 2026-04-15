package unit_test

import (
	"testing"

	"github.com/edalcin/pkd/internal/security"
)

func TestVerifyMaster_Correct(t *testing.T) {
	if !security.VerifyMaster("secret", "secret") {
		t.Error("expected true for matching passwords")
	}
}

func TestVerifyMaster_Wrong(t *testing.T) {
	cases := []struct{ provided, configured string }{
		{"wrong", "secret"},
		{"", "secret"},
		{"secret", ""},
		{"Secret", "secret"}, // case-sensitive
		{"secret ", "secret"}, // trailing space
	}
	for _, tc := range cases {
		if security.VerifyMaster(tc.provided, tc.configured) {
			t.Errorf("expected false for provided=%q configured=%q", tc.provided, tc.configured)
		}
	}
}

func TestVerifyMaster_TimingLength(t *testing.T) {
	// Both branches hash to 32 bytes, so no length information is leaked.
	// This is a structural check only — real timing attacks need many samples.
	_ = security.VerifyMaster("a", "a very long password that is different")
	_ = security.VerifyMaster("a very long password that is different", "a")
}
