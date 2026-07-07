package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

const docEncKeyContext = "pkd-doc-enc-v1:"

// ErrDecrypt is returned when ciphertext cannot be decrypted (wrong key/tamper).
var ErrDecrypt = errors.New("decrypt failed")

// DeriveDocKey derives a 32-byte AES-256 key from the master password.
// ponytail: SHA-256 KDF, no external dep; master password is a strong single-user
// secret. Upgrade to scrypt/argon2 (golang.org/x/crypto) only if weak passwords
// become part of the threat model.
func DeriveDocKey(masterPassword string) []byte {
	sum := sha256.Sum256([]byte(docEncKeyContext + masterPassword))
	return sum[:]
}

// EncryptDoc returns base64(nonce||ciphertext) under AES-256-GCM.
func EncryptDoc(plaintext string, key []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptDoc reverses EncryptDoc.
func DecryptDoc(encoded string, key []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrDecrypt
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrDecrypt
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
