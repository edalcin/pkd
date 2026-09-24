package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// memoryRequest is the body of POST and PATCH /api/memories. Pointers tell
// PATCH which fields were sent. Content is HTML, sanitized like /api/import.
type memoryRequest struct {
	Title          *string            `json:"title"`
	Content        *string            `json:"content"`
	Tags           *[]string          `json:"tags"`
	Attachments    []importAttachment `json:"attachments"`
	Date           *store.MemoryDate  `json:"date"`
	IdempotencyKey string             `json:"idempotency_key"`
}

const maxIdempotencyKeyLen = 200

// tokenOrSession admits the PKD_IMPORT_TOKEN bearer (agents) or a logged-in
// session (UI). A bearer that does not match is 401 — never a fallthrough to
// the cookie, because CSRF skips every bearer request.
func (s *Server) tokenOrSession(next http.Handler) http.Handler {
	session := AuthRequired(s.sessions)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			session.ServeHTTP(w, r)
			return
		}
		if s.cfg.ImportToken == "" ||
			subtle.ConstantTimeCompare([]byte(auth), []byte("Bearer "+s.cfg.ImportToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleListMemories serves GET /api/memories (session only): the MC tree,
// already in display order.
func (s *Server) handleListMemories() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := s.docs.ListMemories()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleCreateMemory serves POST /api/memories. The same idempotency_key
// returns the existing Memória with 200 instead of creating a duplicate.
func (s *Server) handleCreateMemory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req memoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		title := ""
		if req.Title != nil {
			title = strings.TrimSpace(*req.Title)
		}
		if title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		if req.Date == nil {
			http.Error(w, "date is required", http.StatusBadRequest)
			return
		}
		if err := req.Date.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		key := strings.TrimSpace(req.IdempotencyKey)
		if len(key) > maxIdempotencyKeyLen {
			http.Error(w, "idempotency_key too long", http.StatusBadRequest)
			return
		}

		doc, created, err := s.docs.CreateMemory(title, *req.Date, key)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !created {
			writeJSON(w, http.StatusOK, doc)
			return
		}

		content := ""
		if req.Content != nil {
			content = *req.Content
		}
		if len(req.Attachments) > 0 {
			block, status, importErr := s.importAttachments(r, doc.ID, req.Attachments)
			if importErr != nil {
				s.rollbackImportedDocument(doc.ID)
				http.Error(w, importErr.Error(), status)
				return
			}
			content += block
		}
		plain := ""
		if content != "" {
			safe := security.SanitizeEditorHTML(content)
			plain = security.ExtractPlainText(safe)
			updated, err := s.docs.Update(doc.ID, doc.Version, doc.Title, safe, plain, doc.Icon)
			if err != nil {
				s.rollbackImportedDocument(doc.ID)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			doc = updated
		}
		var tags []string
		if req.Tags != nil {
			tags = *req.Tags
			if err := s.tags.SetDocumentTags(doc.ID, tags); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		s.reindexMemory(doc.ID, doc.Title, plain, tags)
		s.writeMemory(w, http.StatusCreated, doc.ID)
	}
}

// handleGetMemory serves GET /api/memories/{memoryID}.
func (s *Server) handleGetMemory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doc, ok := s.memoryFromPath(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, withheldIfEncrypted(doc))
	}
}

// handlePatchMemory serves PATCH /api/memories/{memoryID}. date replaces the
// whole Data da Memória; the memory_id never changes (ADR-007).
func (s *Server) handlePatchMemory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doc, ok := s.memoryFromPath(w, r)
		if !ok {
			return
		}
		var req memoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// Validate everything before the first write.
		if req.Date != nil {
			if err := req.Date.Validate(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		title := doc.Title
		if req.Title != nil {
			title = strings.TrimSpace(*req.Title)
			if title == "" {
				http.Error(w, "title must not be empty", http.StatusBadRequest)
				return
			}
		}
		textChange := req.Title != nil || req.Content != nil
		if textChange && doc.Encrypted {
			http.Error(w, "memory is encrypted", http.StatusConflict)
			return
		}

		if req.Date != nil {
			if _, err := s.docs.UpdateMemoryDate(doc.ID, *req.Date); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		bodyHTML, bodyText := doc.BodyHTML, doc.BodyText
		if req.Content != nil {
			bodyHTML = security.SanitizeEditorHTML(*req.Content)
			bodyText = security.ExtractPlainText(bodyHTML)
		}
		if textChange {
			_, err := s.docs.Update(doc.ID, doc.Version, title, bodyHTML, bodyText, doc.Icon)
			var dup *store.DuplicateTitleError
			switch {
			case errors.As(err, &dup):
				http.Error(w, "title already used by another document", http.StatusConflict)
				return
			case errors.Is(err, store.ErrLocked):
				http.Error(w, "locked", http.StatusForbidden)
				return
			case errors.Is(err, store.ErrVersionConflict):
				http.Error(w, "version conflict", http.StatusConflict)
				return
			case err != nil:
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		tags := doc.Tags
		if req.Tags != nil {
			tags = *req.Tags
			if err := s.tags.SetDocumentTags(doc.ID, tags); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if (textChange || req.Tags != nil) && !doc.Encrypted {
			s.reindexMemory(doc.ID, title, bodyText, tags)
		}
		s.writeMemory(w, http.StatusOK, doc.ID)
	}
}

func (s *Server) memoryFromPath(w http.ResponseWriter, r *http.Request) (*model.Document, bool) {
	mid, ok := store.NormalizeMemoryID(chi.URLParam(r, "memoryID"))
	if !ok {
		http.Error(w, "invalid memory id", http.StatusBadRequest)
		return nil, false
	}
	doc, err := s.docs.GetByMemoryID(mid)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil, false
	}
	return doc, true
}

// reindexMemory keeps FTS and embeddings current after an API write; the
// editor path does the same in handleUpdateDocument.
func (s *Server) reindexMemory(id int64, title, plain string, tags []string) {
	_ = s.search.IndexDoc(id, title, plain, tags)
	s.embedder.notify()
}

func (s *Server) writeMemory(w http.ResponseWriter, status int, id int64) {
	doc, err := s.docs.GetByID(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, status, withheldIfEncrypted(doc))
}

// withheldIfEncrypted never returns ciphertext to API clients.
func withheldIfEncrypted(doc *model.Document) *model.Document {
	if doc.Encrypted {
		doc.BodyHTML, doc.BodyText = "", ""
		doc.EncryptedLocked = true
	}
	return doc
}
