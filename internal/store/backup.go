package store

import (
	"database/sql"
	"fmt"
	"os"
)

// BackupStore handles live-consistent SQLite backup and restore.
type BackupStore struct {
	db     *sql.DB
	dbPath string
}

// NewBackupStore wraps db and the path of the primary database file.
func NewBackupStore(db *sql.DB, dbPath string) *BackupStore {
	return &BackupStore{db: db, dbPath: dbPath}
}

// Backup produces a live-consistent snapshot of the database at destPath
// using SQLite's VACUUM INTO command. This works safely even with concurrent
// WAL-mode writers because VACUUM INTO reads a consistent snapshot.
func (s *BackupStore) Backup(destPath string) error {
	_, err := s.db.Exec("VACUUM INTO ?", destPath)
	if err != nil {
		return fmt.Errorf("backup: VACUUM INTO failed: %w", err)
	}
	return nil
}

// Restore replaces the primary database file with srcPath. The caller is
// responsible for stopping all writes before calling Restore and reopening
// the DB afterwards. This implementation swaps the file in place and returns;
// the caller must call store.Open again.
//
// This function:
//  1. Validates srcPath is a readable SQLite file (contains magic bytes).
//  2. Copies srcPath → dbPath atomically by writing to a temp file first.
func (s *BackupStore) Restore(srcPath string) error {
	// Basic validation: first 16 bytes of a valid SQLite file start with
	// "SQLite format 3\x00" (https://www.sqlite.org/fileformat.html)
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("restore: open source: %w", err)
	}
	magic := make([]byte, 16)
	_, err = f.Read(magic)
	f.Close()
	if err != nil || string(magic) != "SQLite format 3\x00" {
		return fmt.Errorf("restore: source is not a valid SQLite database")
	}

	// Close the current DB connection pool before swapping files.
	// The caller will reopen it.
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("restore: close db: %w", err)
	}

	// Copy src → dst via temp file for atomicity
	tmp := s.dbPath + ".restore.tmp"
	if err := copyFile(srcPath, tmp); err != nil {
		return fmt.Errorf("restore: copy to temp: %w", err)
	}
	if err := os.Rename(tmp, s.dbPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("restore: rename: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			break
		}
	}
	return out.Sync()
}
