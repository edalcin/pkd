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
