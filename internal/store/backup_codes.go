package store

import (
	"database/sql"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

// BackupCodeStore manages single-use 2FA recovery codes.
type BackupCodeStore struct{ db *sql.DB }

func NewBackupCodeStore(db *sql.DB) *BackupCodeStore { return &BackupCodeStore{db: db} }

// Replace atomically deletes every existing code and inserts the SHA-256 hash
// of each normalized plaintext code. The plaintext is never stored.
func (s *BackupCodeStore) Replace(codes []string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return WithTx(s.db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM backup_codes`); err != nil {
			return err
		}
		for _, c := range codes {
			if _, err := tx.Exec(
				`INSERT INTO backup_codes(code_hash, created_at) VALUES(?, ?)`,
				security.HashSHA256(security.NormalizeBackupCode(c)), now); err != nil {
				return err
			}
		}
		return nil
	})
}

// Consume deletes the matching unused code (single-use). Returns true iff a
// code existed and was consumed. Empty/invalid input returns (false, nil).
func (s *BackupCodeStore) Consume(code string) (bool, error) {
	norm := security.NormalizeBackupCode(code)
	if norm == "" {
		return false, nil
	}
	res, err := s.db.Exec(`DELETE FROM backup_codes WHERE code_hash = ?`,
		security.HashSHA256(norm))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Count returns the number of remaining (unused) codes.
func (s *BackupCodeStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM backup_codes`).Scan(&n)
	return n, err
}
