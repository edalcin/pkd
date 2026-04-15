package store

import (
	"database/sql"
	"fmt"
)

// WithTx runs fn inside a database transaction. If fn returns nil the
// transaction is committed; otherwise it is rolled back and the error is
// returned. The caller's error is always preserved — rollback errors are
// logged but do not replace the original error.
func WithTx(db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("fn error: %w; rollback error: %v", err, rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
