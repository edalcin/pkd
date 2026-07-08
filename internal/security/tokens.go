package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// NewToken returns n cryptographically random bytes encoded as base64url
// (unpadded). Panics if crypto/rand fails (OS-level error).
func NewToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("security.NewToken: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashSHA256 returns the SHA-256 hash of s as a raw byte slice.
// Used for storing share-link tokens: plaintext returned once, hash stored.
func HashSHA256(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// NewNumericCode returns an n-digit numeric 2FA code (leading zeros kept),
// generated with crypto/rand. Panics if crypto/rand fails.
func NewNumericCode(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("security.NewNumericCode: crypto/rand failed: " + err.Error())
	}
	for i := range b {
		b[i] = digits[int(b[i])%10] // modulo bias negligible for a 2FA code
	}
	return string(b)
}

// backupCodeAlphabet excludes ambiguous glyphs (I, L, O, U, 0, 1). 30 symbols.
const backupCodeAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
const backupCodeLen = 10 // 30^10 ≈ 49 bits; grouped 5-5 for readability

// NewBackupCode returns a human-friendly single-use recovery code such as
// "ABCDE-FGHJK". Uses rejection sampling (reject bytes >= 240 = 8*30) so the
// alphabet is uniform — unlike NewNumericCode, backup codes bypass e-mail so
// entropy must not be biased. Panics if crypto/rand fails.
func NewBackupCode() string {
	b := make([]byte, backupCodeLen)
	var buf [1]byte
	for i := 0; i < backupCodeLen; {
		if _, err := rand.Read(buf[:]); err != nil {
			panic("security.NewBackupCode: crypto/rand failed: " + err.Error())
		}
		if buf[0] >= 240 { // avoid modulo bias
			continue
		}
		b[i] = backupCodeAlphabet[int(buf[0])%len(backupCodeAlphabet)]
		i++
	}
	return string(b[:5]) + "-" + string(b[5:])
}

// NormalizeBackupCode uppercases s and drops every character outside the
// backup-code alphabet (dashes, spaces), so a code verifies regardless of the
// separators/case the user typed.
func NormalizeBackupCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if strings.ContainsRune(backupCodeAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
