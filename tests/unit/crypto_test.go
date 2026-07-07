package unit_test

import (
	"errors"
	"testing"

	"github.com/edalcin/pkd/internal/security"
)

func TestEncryptDecryptDoc_RoundTrip(t *testing.T) {
	key := security.DeriveDocKey("right")
	cases := map[string]string{
		"empty":        "",
		"short_ascii":  "hello",
		"unicode_html": "<p>Olá ção 🐶</p>",
	}
	for name, plaintext := range cases {
		t.Run(name, func(t *testing.T) {
			ct, err := security.EncryptDoc(plaintext, key)
			if err != nil {
				t.Fatalf("EncryptDoc(%q) error: %v", plaintext, err)
			}
			got, err := security.DecryptDoc(ct, key)
			if err != nil {
				t.Fatalf("DecryptDoc error: %v", err)
			}
			if got != plaintext {
				t.Errorf("round-trip mismatch: want %q, got %q", plaintext, got)
			}
		})
	}
}

func TestEncryptDoc_CiphertextDiffersFromPlaintextAndIsRandomized(t *testing.T) {
	key := security.DeriveDocKey("right")
	plaintext := "<p>Olá ção 🐶</p>"

	ct1, err := security.EncryptDoc(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptDoc error: %v", err)
	}
	ct2, err := security.EncryptDoc(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptDoc error: %v", err)
	}

	if ct1 == plaintext || ct2 == plaintext {
		t.Errorf("ciphertext must never equal plaintext: ct1=%q ct2=%q plaintext=%q", ct1, ct2, plaintext)
	}
	if ct1 == ct2 {
		t.Errorf("encrypting the same plaintext twice must produce different ciphertext (random nonce), got identical: %q", ct1)
	}
}

func TestDecryptDoc_WrongKeyFails(t *testing.T) {
	rightKey := security.DeriveDocKey("right")
	wrongKey := security.DeriveDocKey("wrong")

	ct, err := security.EncryptDoc("secret body", rightKey)
	if err != nil {
		t.Fatalf("EncryptDoc error: %v", err)
	}

	got, err := security.DecryptDoc(ct, wrongKey)
	if !errors.Is(err, security.ErrDecrypt) {
		t.Fatalf("expected ErrDecrypt for wrong key, got err=%v", err)
	}
	if got != "" {
		t.Errorf("expected empty plaintext on decrypt failure, got %q", got)
	}
}

func TestDecryptDoc_GarbageInputFails(t *testing.T) {
	key := security.DeriveDocKey("right")
	cases := map[string]string{
		"not_base64":         "not-valid-base64!!!",
		"empty_string":       "",
		"valid_base64_short": "YWJj", // decodes to "abc", shorter than GCM nonce size
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := security.DecryptDoc(input, key)
			if !errors.Is(err, security.ErrDecrypt) {
				t.Fatalf("expected ErrDecrypt for input %q, got err=%v", input, err)
			}
			if got != "" {
				t.Errorf("expected empty plaintext on decrypt failure, got %q", got)
			}
		})
	}
}

func TestDeriveDocKey(t *testing.T) {
	k1 := security.DeriveDocKey("right")
	k2 := security.DeriveDocKey("right")
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d bytes", len(k1))
	}
	if string(k1) != string(k2) {
		t.Errorf("DeriveDocKey must be deterministic: same password produced different keys")
	}

	kOther := security.DeriveDocKey("wrong")
	if string(k1) == string(kOther) {
		t.Errorf("DeriveDocKey must differ for different passwords")
	}
}
