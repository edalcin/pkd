package server

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	throttleMaxFailures = 5
	throttleLockoutDur  = 30 * time.Minute
)

type ipState struct {
	failures  int
	lockedAt  time.Time
}

// Throttle tracks failed-login attempts per IP and enforces a 30-minute
// lockout after 5 consecutive failures. It is attached to the login route
// only (not the full middleware chain).
//
// When PKD_TRUST_PROXY_HEADERS is enabled, the real IP is read from the
// rightmost non-private entry in X-Forwarded-For instead of RemoteAddr.
type Throttle struct {
	mu          sync.Mutex
	states      map[string]*ipState
	trustProxy  bool
}

// ExportNewThrottle is the same as NewThrottle but exported for unit tests.
// Use NewThrottle in production code.
func ExportNewThrottle(trustProxy bool) *Throttle { return NewThrottle(trustProxy) }

// ExportReset clears all throttle state. For use in integration tests only.
func (t *Throttle) ExportReset() {
	t.mu.Lock()
	t.states = make(map[string]*ipState)
	t.mu.Unlock()
}

// NewThrottle creates a Throttle. trustProxy should be set to cfg.TrustProxyHeaders.
func NewThrottle(trustProxy bool) *Throttle {
	t := &Throttle{
		states:     make(map[string]*ipState),
		trustProxy: trustProxy,
	}
	go t.sweepLoop()
	return t
}

// Allow returns true if the IP is not currently locked out.
func (t *Throttle) Allow(r *http.Request) bool {
	ip := t.realIP(r)
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.states[ip]
	if !ok {
		return true
	}
	if state.failures < throttleMaxFailures {
		return true
	}
	return time.Since(state.lockedAt) >= throttleLockoutDur
}

// RecordFailure increments the failure counter for the request's IP.
func (t *Throttle) RecordFailure(r *http.Request) {
	ip := t.realIP(r)
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.states[ip]
	if !ok {
		state = &ipState{}
		t.states[ip] = state
	}
	state.failures++
	if state.failures == throttleMaxFailures {
		state.lockedAt = time.Now()
	}
}

// RecordSuccess resets the failure counter for the request's IP.
func (t *Throttle) RecordSuccess(r *http.Request) {
	ip := t.realIP(r)
	t.mu.Lock()
	delete(t.states, ip)
	t.mu.Unlock()
}

// RetryAfter returns the number of seconds until the lockout expires.
func (t *Throttle) RetryAfter(r *http.Request) int {
	ip := t.realIP(r)
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.states[ip]
	if !ok || state.failures < throttleMaxFailures {
		return 0
	}
	remaining := throttleLockoutDur - time.Since(state.lockedAt)
	if remaining < 0 {
		return 0
	}
	return int(remaining.Seconds()) + 1
}

func (t *Throttle) realIP(r *http.Request) string {
	if t.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the rightmost IP that is not trusted (the last client hop)
			// Simple implementation: take the last entry
			last := xff
			if i := len(xff) - 1; i >= 0 {
				for i >= 0 && xff[i] != ',' {
					i--
				}
				last = xff[i+1:]
			}
			ip := net.ParseIP(trimSpaces(last))
			if ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func trimSpaces(s string) string {
	start, end := 0, len(s)-1
	for start <= end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end >= start && (s[end] == ' ' || s[end] == '\t') {
		end--
	}
	return s[start : end+1]
}

// sweepLoop purges expired lockouts every minute to prevent unbounded growth.
func (t *Throttle) sweepLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-throttleLockoutDur)
		t.mu.Lock()
		for ip, state := range t.states {
			if state.failures >= throttleMaxFailures && state.lockedAt.Before(cutoff) {
				delete(t.states, ip)
			}
		}
		t.mu.Unlock()
	}
}

// ThrottleHeader writes the Retry-After header and returns 429.
func ThrottleHeader(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	http.Error(w, "too many failed login attempts; try again later", http.StatusTooManyRequests)
}
