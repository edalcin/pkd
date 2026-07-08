package server

import (
	"sync"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

const (
	challengeTTL       = 10 * time.Minute
	challengeMaxTries  = 5
	codeDigits         = 6
	deviceCookieName   = "pkd_device"
	deviceCookieMaxAge = 315360000 // 10 years, seconds
)

type challenge struct {
	kind      string // "login" | "doc" | "backup"
	codeHash  []byte
	sessionID string // set for kind=="doc"
	docID     int64  // set for kind=="doc"
	expiresAt time.Time
	tries     int
}

type challengeStore struct {
	mu sync.Mutex
	m  map[string]*challenge
}

func newChallengeStore() *challengeStore {
	c := &challengeStore{m: make(map[string]*challenge)}
	go c.sweepLoop()
	return c
}

// create stores a challenge for code and returns its opaque id.
func (c *challengeStore) create(kind, code, sessionID string, docID int64) string {
	id := security.NewToken(24)
	c.mu.Lock()
	c.m[id] = &challenge{kind: kind, codeHash: security.HashSHA256(code), sessionID: sessionID, docID: docID, expiresAt: time.Now().Add(challengeTTL)}
	c.mu.Unlock()
	return id
}

// verify consumes and validates a challenge by comparing the emailed code.
func (c *challengeStore) verify(id, code string) (*challenge, bool) {
	return c.verifyFunc(id, func(ch *challenge) bool {
		return security.ConstantTimeEqualBytes(ch.codeHash, security.HashSHA256(code))
	})
}

// verifyFunc validates the challenge (existence, expiry, try count) and consumes
// it iff match reports the supplied secret correct. match runs under the store
// lock; keep it fast. A failing match keeps the challenge (its try counter was
// already incremented, so challengeMaxTries still caps attempts).
// ponytail: match may hit the DB (backup-code check) under the lock — this store
// is contended only by rare 2FA verifies + the once-a-minute sweep, so fine.
func (c *challengeStore) verifyFunc(id string, match func(*challenge) bool) (*challenge, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.m[id]
	if !ok || time.Now().After(ch.expiresAt) {
		delete(c.m, id)
		return nil, false
	}
	ch.tries++
	if ch.tries > challengeMaxTries {
		delete(c.m, id)
		return nil, false
	}
	if !match(ch) {
		return nil, false
	}
	delete(c.m, id) // single-use
	return ch, true
}

func (c *challengeStore) sweepLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for id, ch := range c.m {
			if now.After(ch.expiresAt) {
				delete(c.m, id)
			}
		}
		c.mu.Unlock()
	}
}

// send2FACode e-mails a code to the configured EMAIL_2FA recipient.
func (s *Server) send2FACode(subject, body string) error {
	return s.email.Send(s.cfg.Email2FA, subject, body)
}
