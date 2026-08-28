package store

import (
	"database/sql"
	"errors"
	"time"
)

// SettingsStore provides key-value persistence backed by the settings table.
type SettingsStore struct {
	db *sql.DB
}

func NewSettingsStore(db *sql.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// Get returns the value for key, or ErrNotFound if absent.
func (s *SettingsStore) Get(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

// Set upserts the value for key.
func (s *SettingsStore) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// AttachmentsBackend returns "local" or "s3".
func (s *SettingsStore) AttachmentsBackend() (string, error) {
	v, err := s.Get("attachments.backend")
	if errors.Is(err, ErrNotFound) {
		return "local", nil
	}
	return v, err
}

// SetAttachmentsBackend persists the active backend name.
func (s *SettingsStore) SetAttachmentsBackend(name string) error {
	return s.Set("attachments.backend", name)
}

// VersionsMaxPerDoc returns the configured per-document version retention limit.
func (s *SettingsStore) VersionsMaxPerDoc() (string, error) {
	v, err := s.Get("versions.max_per_doc")
	if errors.Is(err, ErrNotFound) {
		return "50", nil
	}
	return v, err
}

// SetVersionsMaxPerDoc persists the version retention limit.
func (s *SettingsStore) SetVersionsMaxPerDoc(value string) error {
	return s.Set("versions.max_per_doc", value)
}
