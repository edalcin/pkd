package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/config"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/sessions"
	"github.com/edalcin/pkd/internal/storage"
	"github.com/edalcin/pkd/internal/store"
)

// TestProtectedAttachmentAccess locks the access rule for the files of a
// protected document: GET /api/attachments/{id} must refuse a session that has
// not passed the e-mail unlock, and must serve the decrypted bytes to one that
// has. Before this gate existed the endpoint served every attachment by ID,
// so a protected document's files were readable while its body stayed locked.
func TestProtectedAttachmentAccess(t *testing.T) {
	db, err := store.Open("file:server_protected_attachment_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	local := storage.NewLocal(t.TempDir())
	s := &Server{
		cfg:         &config.Config{Password: "master-pw"},
		docs:        store.NewDocumentStore(db),
		attachments: store.NewAttachmentStore(db, local, nil),
		sessions:    sessions.New(60),
	}

	doc, err := s.docs.Create(nil, "Doc Protegido Com Anexo")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	plain := []byte("relatorio confidencial")
	att, err := s.attachments.CreateFile(context.Background(), local, doc.ID,
		"relatorio.txt", "text/plain", "", bytes.NewReader(plain), 1<<20)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	// Protect: cipher the files, then the body — the same order the handler uses.
	key := security.DeriveDocKey(s.cfg.Password)
	if _, err := s.attachments.EncryptDocumentFiles(context.Background(), doc.ID, key); err != nil {
		t.Fatalf("EncryptDocumentFiles: %v", err)
	}
	cipherHTML, err := security.EncryptDoc("<p>segredo</p>", key)
	if err != nil {
		t.Fatalf("EncryptDoc: %v", err)
	}
	if _, err := s.docs.Protect(doc.ID, cipherHTML); err != nil {
		t.Fatalf("Protect: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/api/attachments/{id}", s.handleGetAttachment())

	get := func(sess *sessions.Session) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/attachments/"+strconv.FormatInt(att.ID, 10), nil)
		if sess != nil {
			req = req.WithContext(context.WithValue(req.Context(), sessionKey, sess))
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	if rec := get(nil); rec.Code != http.StatusForbidden {
		t.Fatalf("no session: want 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	locked := s.sessions.Create("127.0.0.1")
	if rec := get(locked); rec.Code != http.StatusForbidden {
		t.Fatalf("locked session: want 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	unlocked := s.sessions.Create("127.0.0.1")
	s.sessions.UnlockDoc(unlocked.ID, doc.ID)
	rec := get(unlocked)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlocked session: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !bytes.Equal(body, plain) {
		t.Fatalf("served body = %q, want the decrypted %q", body, plain)
	}
}
