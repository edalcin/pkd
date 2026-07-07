package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
