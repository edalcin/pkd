package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/edalcin/pkd/internal/store"
)

// The pool has a single connection (store.Open); a health check that does
// not release it hangs every later query, including the next health check.
func TestHealthReleasesConnection(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "pkd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &healthHandler{db: db}
	for i := range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: status %d", i, rec.Code)
		}
	}
	if inUse := db.Stats().InUse; inUse != 0 {
		t.Fatalf("connections still in use after health checks: %d", inUse)
	}
}
