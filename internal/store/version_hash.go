package store

import (
	"crypto/sha256"
	"encoding/hex"
)

// documentContentSHA256 returns a hex-encoded SHA-256 of the document fields
// that trigger a new version snapshot. The "v1\n" prefix and null-byte separators
// prevent hash collisions across field boundaries.
func documentContentSHA256(title, bodyHTML, icon string) string {
	h := sha256.New()
	h.Write([]byte("v1\n"))
	h.Write([]byte(title))
	h.Write([]byte{0})
	h.Write([]byte(icon))
	h.Write([]byte{0})
	h.Write([]byte(bodyHTML))
	return hex.EncodeToString(h.Sum(nil))
}
