package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/store"
)

// TestCaptureCreatesNote proves /api/capture creates a Nota (is_note=1),
// tagged #captura, instead of a root Documento.
func TestCaptureCreatesNote(t *testing.T) {
	db, err := store.Open("file:server_capture_note_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	s := &Server{
		docs: store.NewDocumentStore(db),
		tags: store.NewTagStore(db),
	}

	r := chi.NewRouter()
	r.Post("/api/capture", s.handleCapture())

	body := strings.NewReader(`{"title":"Artigo interessante","content":"<p>corpo</p>"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/capture", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	doc, err := s.docs.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !doc.IsNote {
		t.Fatalf("captured document must be a Nota (is_note=1), got IsNote=false")
	}
	if doc.ParentID != nil {
		t.Fatalf("Nota must have no parent, got %v", doc.ParentID)
	}
	found := false
	for _, tag := range doc.Tags {
		if tag == "captura" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Nota missing #captura tag, got %v", doc.Tags)
	}
}

// TestCaptureIdempotencyKey proves that POSTing /api/capture twice with the
// same idempotency_key creates only one Nota: the replay returns 200 with
// the existing Nota instead of a duplicate 201.
func TestCaptureIdempotencyKey(t *testing.T) {
	db, err := store.Open("file:server_capture_idem_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	s := &Server{
		docs: store.NewDocumentStore(db),
		tags: store.NewTagStore(db),
	}

	r := chi.NewRouter()
	r.Post("/api/capture", s.handleCapture())

	post := func() *httptest.ResponseRecorder {
		body := strings.NewReader(`{"title":"Repetido","content":"<p>x</p>","idempotency_key":"share-123"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/capture", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	first := post()
	if first.Code != http.StatusCreated {
		t.Fatalf("first capture: want 201, got %d (%s)", first.Code, first.Body.String())
	}

	second := post()
	if second.Code != http.StatusOK {
		t.Fatalf("replayed capture: want 200, got %d (%s)", second.Code, second.Body.String())
	}

	notes, err := s.docs.ListNotes(nil, false)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("want exactly 1 Nota after idempotent replay, got %d", len(notes))
	}
}
