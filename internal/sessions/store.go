package sessions

import (
	"database/sql"
	"sync"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

// Session holds an authenticated session.
type Session struct {
	ID         string
	IP         string
	CreatedAt  time.Time
	LastSeenAt time.Time
	unlocked   map[int64]struct{} // in-memory only: doc IDs unlocked this session
}

// Store is a thread-safe session store backed by SQLite for persistence across
// server restarts. Pass nil (or omit) for db to use memory-only mode (tests).
type Store struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	idleTimeout time.Duration
	db          *sql.DB
}

// New creates a Store that expires sessions idle for longer than idleMinutes.
// The optional db parameter enables SQLite persistence: sessions survive server
// restarts and are loaded back on startup. Without db, sessions are memory-only.
func New(idleMinutes int, db ...*sql.DB) *Store {
	var sqlDB *sql.DB
	if len(db) > 0 {
		sqlDB = db[0]
	}
	s := &Store{
		sessions:    make(map[string]*Session),
		idleTimeout: time.Duration(idleMinutes) * time.Minute,
		db:          sqlDB,
	}
	s.loadFromDB()
	go s.sweepLoop()
	return s
}

// loadFromDB loads non-expired sessions from SQLite into memory on startup.
func (s *Store) loadFromDB() {
	if s.db == nil {
		return
	}
	cutoff := time.Now().Add(-s.idleTimeout).Unix()
	rows, err := s.db.Query(
		"SELECT id, ip, created_at, last_seen_at FROM sessions WHERE last_seen_at > ?",
		cutoff,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sess Session
		var createdAt, lastSeenAt int64
		if err := rows.Scan(&sess.ID, &sess.IP, &createdAt, &lastSeenAt); err != nil {
			continue
		}
		sess.CreatedAt = time.Unix(createdAt, 0)
		sess.LastSeenAt = time.Unix(lastSeenAt, 0)
		s.sessions[sess.ID] = &sess
	}
}

// Create allocates a new session for the given IP and returns it.
func (s *Store) Create(ip string) *Session {
	sess := &Session{
		ID:         security.NewToken(32),
		IP:         ip,
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	if s.db != nil {
		_, _ = s.db.Exec(
			"INSERT OR IGNORE INTO sessions(id, ip, created_at, last_seen_at) VALUES(?,?,?,?)",
			sess.ID, sess.IP, sess.CreatedAt.Unix(), sess.LastSeenAt.Unix(),
		)
	}
	return sess
}

// Get returns the session for id, or (nil, false) if it does not exist or
// has been evicted.
func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	return sess, ok
}

// UnlockDoc marks docID decrypted for sessionID (in-memory; gone on session
// delete/expiry/restart).
func (s *Store) UnlockDoc(sessionID string, docID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		if sess.unlocked == nil {
			sess.unlocked = make(map[int64]struct{})
		}
		sess.unlocked[docID] = struct{}{}
	}
}

// IsDocUnlocked reports whether docID was unlocked in sessionID.
func (s *Store) IsDocUnlocked(sessionID string, docID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sess, ok := s.sessions[sessionID]; ok {
		_, u := sess.unlocked[docID]
		return u
	}
	return false
}

// Touch updates the last-seen time for the given session.
// The DB write is async (fire-and-forget) to avoid blocking request handling.
func (s *Store) Touch(id string) {
	now := time.Now()
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.LastSeenAt = now
	}
	s.mu.Unlock()
	if s.db != nil {
		go func() {
			_, _ = s.db.Exec("UPDATE sessions SET last_seen_at=? WHERE id=?", now.Unix(), id)
		}()
	}
}

// Delete removes the session (used for logout).
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	if s.db != nil {
		_, _ = s.db.Exec("DELETE FROM sessions WHERE id=?", id)
	}
}

// Reset clears all sessions and returns a fresh Store with the same idle
// timeout. Pass the new DB when called after a database restore so the new
// store uses the restored database.
func (s *Store) Reset(newDB ...*sql.DB) *Store {
	s.mu.Lock()
	idleTimeout := s.idleTimeout
	db := s.db
	s.mu.Unlock()
	if len(newDB) > 0 {
		db = newDB[0]
	}
	if db != nil {
		_, _ = db.Exec("DELETE FROM sessions")
	}
	return New(int(idleTimeout.Minutes()), db)
}

// sweepLoop evicts idle sessions every minute.
func (s *Store) sweepLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.sweep()
	}
}

func (s *Store) sweep() {
	cutoff := time.Now().Add(-s.idleTimeout)
	var expired []string
	s.mu.Lock()
	for id, sess := range s.sessions {
		if sess.LastSeenAt.Before(cutoff) {
			expired = append(expired, id)
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
	if s.db != nil {
		for _, id := range expired {
			_, _ = s.db.Exec("DELETE FROM sessions WHERE id=?", id)
		}
	}
}
