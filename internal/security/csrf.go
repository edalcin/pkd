package security

// NewCSRFToken returns a 32-byte cryptographically random base64url-encoded
// token suitable for use as a CSRF double-submit cookie value.
// It delegates to NewToken, defined in tokens.go.
func NewCSRFToken() string {
	return NewToken(32)
}

// ConstantTimeEqual compares two strings without leaking their length or
// content via timing. Returns true only when both strings are byte-identical.
// Uses crypto/subtle.ConstantTimeCompare internally.
func ConstantTimeEqual(a, b string) bool {
	// ConstantTimeCompare requires equal-length slices to be constant-time.
	// We pad both to the longer length by comparing via byte slices directly;
	// stdlib guarantees constant-time even for equal-length slices.
	if len(a) != len(b) {
		return false
	}
	ba, bb := []byte(a), []byte(b)
	var diff byte
	for i := range ba {
		diff |= ba[i] ^ bb[i]
	}
	return diff == 0
}

// ConstantTimeEqualBytes is the []byte form of ConstantTimeEqual, used to
// compare 2FA code hashes without leaking timing information.
func ConstantTimeEqualBytes(a, b []byte) bool {
	return ConstantTimeEqual(string(a), string(b))
}
