package sessions

import (
	"sync"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

// Session holds an authenticated session. Sessions are stored in memory only
// and are lost on server restart (requiring the user to log in again).
type Session struct {
	ID         string
	IP         string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// Store is a thread-safe in-memory session store with idle-timeout eviction.
type Store struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	idleTimeout time.Duration
}

// New creates a Store that expires sessions idle for longer than idleMinutes.
func New(idleMinutes int) *Store {
	s := &Store{
		sessions:    make(map[string]*Session),
		idleTimeout: time.Duration(idleMinutes) * time.Minute,
	}
	go s.sweepLoop()
	return s
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

// Touch updates the last-seen time for the given session.
func (s *Store) Touch(id string) {
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.LastSeenAt = time.Now()
	}
	s.mu.Unlock()
}

// Delete removes the session (used for logout).
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// Reset clears all sessions and returns a fresh Store with the same idle timeout.
// Used after a database restore to force all users to re-authenticate.
func (s *Store) Reset() *Store {
	s.mu.Lock()
	idleTimeout := s.idleTimeout
	s.mu.Unlock()
	return New(int(idleTimeout.Minutes()))
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
	s.mu.Lock()
	for id, sess := range s.sessions {
		if sess.LastSeenAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}
