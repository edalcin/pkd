package server

import (
	"container/list"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edalcin/pkd/internal/backup"
	"github.com/google/uuid"
)

// Job tracks the lifecycle of a long-running backup or restore.
//
// State transitions: running -> succeeded | failed. There is no resume path —
// if the application crashes, the job is lost and any temp objects it created
// are removed by the next startup sweep.
type Job struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`         // "backup" | "migrate" | "reconcile" | "cleanup"
	BackendKind  string    `json:"backend_kind"` // "local" | "s3"
	State        string    `json:"state"`        // "running" | "succeeded" | "failed"
	Processed    int64     `json:"processed"`
	Total        int64     `json:"total"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
	AdminUserID  int64     `json:"admin_user_id"`
	ErrorMessage string    `json:"error_message,omitempty"`
	DownloadURL  string    `json:"download_url,omitempty"`
	URLExpiresAt time.Time `json:"url_expires_at,omitempty"`
	SizeBytes    int64     `json:"size_bytes"`

	// TempKey is the object key (S3) or file path (local) of the in-progress
	// archive. Internal — never serialized.
	TempKey string `json:"-"`

	// Restore is non-nil for restore jobs and carries per-entry outcomes.
	Restore *RestoreSummary `json:"restore,omitempty"`

	// StorageOp is non-nil for migrate/reconcile/cleanup jobs and carries the
	// final operation counters once the job completes.
	StorageOp *StorageOpSummary `json:"storage_op,omitempty"`
}

// StorageOpSummary carries the result of a migrate, reconcile, or cleanup job.
// Processed/Total on the parent Job give live progress; this struct holds the
// final audit counters set once the goroutine finishes.
type StorageOpSummary struct {
	TotalFound int64    `json:"total_found"`
	Succeeded  int64    `json:"succeeded"` // copied | fixed | removed
	Errors     []string `json:"errors,omitempty"`
}

// RestoreSummary aggregates counters and skipped entries from a restore job.
// Exposed in the job snapshot so the admin UI can render an audit panel.
type RestoreSummary struct {
	Written       int64                  `json:"written"`
	Kept          int64                  `json:"kept"`
	SkippedOrphan int64                  `json:"skipped_orphan"`
	HashMismatch  int64                  `json:"hash_mismatch"`
	Skipped       []backup.SkippedEntry  `json:"skipped,omitempty"`
}

// ErrJobInFlight is returned by BackupJobManager.Start when another job is
// already running for the requested backend (FR-015).
var ErrJobInFlight = errors.New("operation already in progress for backend")

// historyCap bounds the size of the completed-job LRU. Older jobs fall out so
// long-running processes do not leak memory.
const historyCap = 50

// BackupJobManager coordinates single-in-flight backup/restore jobs per
// storage backend kind, and keeps a small history of finished jobs for status
// polling after completion.
type BackupJobManager struct {
	mu       sync.Mutex
	active   map[string]*Job // key = backendKind, at most one running job
	history  map[string]*list.Element
	lruOrder *list.List // values are *Job, front=most recent
}

// NewBackupJobManager returns an empty manager.
func NewBackupJobManager() *BackupJobManager {
	return &BackupJobManager{
		active:   make(map[string]*Job),
		history:  make(map[string]*list.Element),
		lruOrder: list.New(),
	}
}

// Start registers a new running job. Returns ErrJobInFlight if another job is
// already running on the same backend.
func (m *BackupJobManager) Start(kind, backendKind string, adminID int64) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, busy := m.active[backendKind]; busy {
		return nil, ErrJobInFlight
	}
	j := &Job{
		ID:          uuid.NewString(),
		Kind:        kind,
		BackendKind: backendKind,
		State:       "running",
		StartedAt:   time.Now().UTC(),
		AdminUserID: adminID,
	}
	m.active[backendKind] = j
	return j, nil
}

// Get returns a snapshot of the job (active or in history). The returned
// pointer is the live job; callers must treat fields as read-only.
func (m *BackupJobManager) Get(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.active {
		if j.ID == id {
			return j, true
		}
	}
	if el, ok := m.history[id]; ok {
		return el.Value.(*Job), true
	}
	return nil, false
}

// Finish moves a running job out of the active set and into the bounded
// history. state must be "succeeded" or "failed". errMsg is recorded only for
// the failed case.
func (m *BackupJobManager) Finish(j *Job, state, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j.State = state
	j.ErrorMessage = errMsg
	j.EndedAt = time.Now().UTC()
	delete(m.active, j.BackendKind)
	if existing, ok := m.history[j.ID]; ok {
		m.lruOrder.MoveToFront(existing)
	} else {
		el := m.lruOrder.PushFront(j)
		m.history[j.ID] = el
		for m.lruOrder.Len() > historyCap {
			oldest := m.lruOrder.Back()
			if oldest == nil {
				break
			}
			old := oldest.Value.(*Job)
			delete(m.history, old.ID)
			m.lruOrder.Remove(oldest)
		}
	}
}

// SetDownloadURL records a presigned URL and its expiry on a job (used after
// a successful S3 backup completes).
func (m *BackupJobManager) SetDownloadURL(id, url string, expiresAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.active {
		if j.ID == id {
			j.DownloadURL = url
			j.URLExpiresAt = expiresAt
			return
		}
	}
	if el, ok := m.history[id]; ok {
		j := el.Value.(*Job)
		j.DownloadURL = url
		j.URLExpiresAt = expiresAt
	}
}

// IncProcessed atomically bumps the processed counter on a job.
// Safe to call from the worker goroutine without holding the manager lock —
// callers update via atomic on a pointer to the field.
func IncProcessed(j *Job, n int64) {
	atomic.AddInt64(&j.Processed, n)
}
