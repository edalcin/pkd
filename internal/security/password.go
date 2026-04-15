package security

import (
	"crypto/sha256"
	"crypto/subtle"
)

// VerifyMaster returns true when provided matches configured using a
// constant-time comparison over SHA-256 digests of both values.
// Hashing first ensures the comparison is always over fixed-length 32-byte
// slices, preventing timing leaks caused by different string lengths.
func VerifyMaster(provided, configured string) bool {
	a := sha256.Sum256([]byte(provided))
	b := sha256.Sum256([]byte(configured))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
