package unit

import (
	"errors"
	"testing"
	"time"

	"github.com/edalcin/pkd/internal/server"
)

func TestJobsSingleInFlightPerBackend(t *testing.T) {
	m := server.NewBackupJobManager()
	j1, err := m.Start("backup", "s3", 0)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := m.Start("backup", "s3", 0); !errors.Is(err, server.ErrJobInFlight) {
		t.Fatalf("second start on same backend should return ErrJobInFlight, got %v", err)
	}
	// Different backend allowed.
	j2, err := m.Start("backup", "local", 0)
	if err != nil {
		t.Fatalf("start on other backend: %v", err)
	}
	if j2.ID == j1.ID {
		t.Errorf("job IDs collided")
	}
}

func TestJobsFinishMovesToHistory(t *testing.T) {
	m := server.NewBackupJobManager()
	j, _ := m.Start("backup", "s3", 0)
	m.Finish(j, "succeeded", "")
	if j.State != "succeeded" {
		t.Errorf("state: got %s", j.State)
	}
	got, ok := m.Get(j.ID)
	if !ok {
		t.Fatalf("get after finish: not found")
	}
	if got.State != "succeeded" {
		t.Errorf("retrieved state: got %s", got.State)
	}
	// After finish, the backend slot is free.
	if _, err := m.Start("backup", "s3", 0); err != nil {
		t.Errorf("restart after finish should succeed, got %v", err)
	}
}

func TestJobsGetUnknown(t *testing.T) {
	m := server.NewBackupJobManager()
	if _, ok := m.Get("00000000-0000-0000-0000-000000000000"); ok {
		t.Errorf("unknown job id should not be found")
	}
}

func TestJobsSetDownloadURL(t *testing.T) {
	m := server.NewBackupJobManager()
	j, _ := m.Start("backup", "s3", 0)
	m.Finish(j, "succeeded", "")
	exp := time.Now().UTC().Add(10 * time.Minute)
	m.SetDownloadURL(j.ID, "https://example.com/zip", exp)
	got, _ := m.Get(j.ID)
	if got.DownloadURL != "https://example.com/zip" {
		t.Errorf("download url: got %q", got.DownloadURL)
	}
	if !got.URLExpiresAt.Equal(exp) {
		t.Errorf("expires_at: got %v want %v", got.URLExpiresAt, exp)
	}
}
