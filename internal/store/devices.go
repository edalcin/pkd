package store

import (
	"database/sql"
	"errors"
	"time"
)

// DeviceStore manages trusted 2FA devices.
type DeviceStore struct{ db *sql.DB }

// NewDeviceStore wraps db.
func NewDeviceStore(db *sql.DB) *DeviceStore { return &DeviceStore{db: db} }

// Trust records a hashed device token as trusted (idempotent upsert).
func (s *DeviceStore) Trust(tokenHash []byte, userAgent string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO trusted_devices(token_hash, user_agent, created_at, last_seen_at)
		VALUES(?,?,?,?)
		ON CONFLICT(token_hash) DO UPDATE SET last_seen_at=excluded.last_seen_at`,
		tokenHash, userAgent, now, now)
	return err
}

// IsTrusted reports whether the hashed token is a known trusted device.
func (s *DeviceStore) IsTrusted(tokenHash []byte) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM trusted_devices WHERE token_hash=?`, tokenHash).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ForgetAll deletes every trusted device (admin revoke).
func (s *DeviceStore) ForgetAll() error {
	_, err := s.db.Exec(`DELETE FROM trusted_devices`)
	return err
}
