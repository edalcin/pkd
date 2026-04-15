package unit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edalcin/pkd/internal/server"
)

// newReq creates a fake request with the given RemoteAddr.
func newReq(addr string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.RemoteAddr = addr + ":12345"
	return r
}

func TestThrottle_AllowBeforeLimit(t *testing.T) {
	thr := server.ExportNewThrottle(false)
	r := newReq("10.0.0.1")

	for i := 0; i < 4; i++ {
		if !thr.Allow(r) {
			t.Fatalf("expected Allow=true after %d failures", i)
		}
		thr.RecordFailure(r)
	}
	// 4 failures — still allowed
	if !thr.Allow(r) {
		t.Error("expected Allow=true at exactly 4 failures")
	}
}

func TestThrottle_LockedAfterFiveFailures(t *testing.T) {
	thr := server.ExportNewThrottle(false)
	r := newReq("10.0.0.2")

	for i := 0; i < 5; i++ {
		thr.RecordFailure(r)
	}
	if thr.Allow(r) {
		t.Error("expected Allow=false after 5 failures")
	}
}

func TestThrottle_ResetOnSuccess(t *testing.T) {
	thr := server.ExportNewThrottle(false)
	r := newReq("10.0.0.3")

	for i := 0; i < 5; i++ {
		thr.RecordFailure(r)
	}
	if thr.Allow(r) {
		t.Error("expected locked before reset")
	}
	thr.RecordSuccess(r)
	if !thr.Allow(r) {
		t.Error("expected Allow=true after RecordSuccess")
	}
}

func TestThrottle_DifferentIPsIndependent(t *testing.T) {
	thr := server.ExportNewThrottle(false)
	r1 := newReq("10.0.1.1")
	r2 := newReq("10.0.1.2")

	for i := 0; i < 5; i++ {
		thr.RecordFailure(r1)
	}
	if thr.Allow(r1) {
		t.Error("expected r1 locked")
	}
	if !thr.Allow(r2) {
		t.Error("expected r2 still allowed")
	}
}
