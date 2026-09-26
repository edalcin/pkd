package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// noteRequest is the body of POST and PATCH /api/notes. Pointers tell PATCH
// which fields were sent. Content is HTML, sanitized like /api/import.
type noteRequest struct {
	Title          *string            `json:"title"`
	Content        *string            `json:"content"`
	Tags           *[]string          `json:"tags"`
	Attachments    []importAttachment `json:"attachments"`
	Favorite       *bool              `json:"favorite"`
	IdempotencyKey string             `json:"idempotency_key"`
	CreatedAt      *string            `json:"created_at"`
	UpdatedAt      *string            `json:"updated_at"`
}

// isBearerRequest reports whether r carries the PKD_IMPORT_TOKEN bearer
// (already validated by tokenOrSession) rather than a session cookie.
// created_at/updated_at are honored only for bearer requests (Q11: datas
// explícitas só via Bearer).
func isBearerRequest(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// parseOptionalRFC3339 parses s (may be nil or empty) as RFC3339; nil in, nil out.
func parseOptionalRFC3339(s *string) (*time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*s))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// handleCreateNote serves POST /api/notes. The same idempotency_key returns
// the existing Nota with 200 instead of creating a duplicate.
func (s *Server) handleCreateNote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req noteRequest
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
		key := strings.TrimSpace(req.IdempotencyKey)
		if len(key) > maxIdempotencyKeyLen {
			http.Error(w, "idempotency_key too long", http.StatusBadRequest)
			return
		}
		favorite := false
		if req.Favorite != nil {
			favorite = *req.Favorite
		}
		var createdAt, updatedAt *time.Time
		if isBearerRequest(r) {
			var err error
			if createdAt, err = parseOptionalRFC3339(req.CreatedAt); err != nil {
				http.Error(w, "invalid created_at", http.StatusBadRequest)
				return
			}
			if updatedAt, err = parseOptionalRFC3339(req.UpdatedAt); err != nil {
				http.Error(w, "invalid updated_at", http.StatusBadRequest)
				return
			}
		}

		doc, created, err := s.docs.CreateNote(title, key, favorite, createdAt, updatedAt)
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
		if updatedAt != nil {
			if err := s.docs.SetUpdatedAt(doc.ID, *updatedAt); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		s.reindexNote(doc.ID, doc.Title, plain, tags)
		s.writeNote(w, http.StatusCreated, doc.ID)
	}
}

// handleGetNote serves GET /api/notes/{id}.
func (s *Server) handleGetNote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doc, ok := s.noteFromPath(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, withheldIfEncrypted(doc))
	}
}

// handlePatchNote serves PATCH /api/notes/{id}.
func (s *Server) handlePatchNote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doc, ok := s.noteFromPath(w, r)
		if !ok {
			return
		}
		var req noteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
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
			http.Error(w, "note is encrypted", http.StatusConflict)
			return
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
			s.reindexNote(doc.ID, title, bodyText, tags)
		}
		s.writeNote(w, http.StatusOK, doc.ID)
	}
}

// handleListNotes serves GET /api/notes (session only): the Notas block
// list, already in display order (Q4), honoring the tag/favorite filters (Q5).
func (s *Server) handleListNotes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tagFilter := r.URL.Query()["tag"]
		favoriteOnly := r.URL.Query().Get("favorite") == "1"
		list, err := s.docs.ListNotes(tagFilter, favoriteOnly)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// convertNoteRequest is the body of POST /api/notes/{id}/convert.
type convertNoteRequest struct {
	To       string            `json:"to"` // "document" | "memory"
	ParentID *int64            `json:"parent_id"`
	BeforeID *int64            `json:"before_id"`
	Date     *store.MemoryDate `json:"date"`
}

// handleConvertNote serves POST /api/notes/{id}/convert (session only, Q3:
// conversão só de ida). "document" places the Nota in the normal tree at
// parent_id/before_id (Q18); "memory" requires date and emits a Memória ID
// (Q17).
func (s *Server) handleConvertNote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req convertNoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		switch req.To {
		case "document":
			doc, err := s.docs.ConvertNoteToDocument(id, req.ParentID, req.BeforeID)
			switch {
			case errors.Is(err, store.ErrNotFound):
				http.Error(w, "not found", http.StatusNotFound)
				return
			case errors.Is(err, store.ErrCircularMove):
				http.Error(w, "circular move not allowed", http.StatusBadRequest)
				return
			case errors.Is(err, store.ErrMemoryHierarchy), errors.Is(err, store.ErrNoteHierarchy):
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			case err != nil:
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, withheldIfEncrypted(doc))
		case "memory":
			if req.Date == nil {
				http.Error(w, "date is required", http.StatusBadRequest)
				return
			}
			if err := req.Date.Validate(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			doc, err := s.docs.ConvertNoteToMemory(id, *req.Date)
			switch {
			case errors.Is(err, store.ErrNotFound):
				http.Error(w, "not found", http.StatusNotFound)
				return
			case err != nil:
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, withheldIfEncrypted(doc))
		default:
			http.Error(w, `to must be "document" or "memory"`, http.StatusBadRequest)
		}
	}
}

func (s *Server) noteFromPath(w http.ResponseWriter, r *http.Request) (*model.Document, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return nil, false
	}
	doc, err := s.docs.GetNote(id)
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

// reindexNote keeps FTS and embeddings current after an API write; the
// editor path does the same in handleUpdateDocument.
func (s *Server) reindexNote(id int64, title, plain string, tags []string) {
	_ = s.search.IndexDoc(id, title, plain, tags)
	s.embedder.notify()
}

func (s *Server) writeNote(w http.ResponseWriter, status int, id int64) {
	doc, err := s.docs.GetByID(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, status, withheldIfEncrypted(doc))
}
