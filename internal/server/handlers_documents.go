package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

func (s *Server) handleCreateDocument() http.HandlerFunc {
	type request struct {
		ParentID *int64 `json:"parent_id"`
		Title    string `json:"title"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			req.Title = "Untitled"
		}
		doc, err := s.docs.Create(req.ParentID, req.Title)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	}
}

func (s *Server) handleGetDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.GetByID(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

func (s *Server) handleUpdateDocument() http.HandlerFunc {
	type request struct {
		Version  int64  `json:"version"`
		Title    string `json:"title"`
		BodyHTML string `json:"body_html"`
		BodyText string `json:"body_text"`
		Icon     string `json:"icon"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Validate icon key against whitelist (T100)
		if !security.ValidateIcon(req.Icon) {
			http.Error(w, "invalid icon", http.StatusBadRequest)
			return
		}
		// Sanitize HTML before storing (FR-042 — XSS prevention)
		safeHTML := security.SanitizeEditorHTML(req.BodyHTML)
		// Derive plain text for FTS5 indexing from the sanitized HTML
		plainText := security.ExtractPlainText(safeHTML)

		docID := id // capture for closure
		docHTML := safeHTML
		doc, err := s.docs.UpdateAndSync(id, req.Version, req.Title, safeHTML, plainText, req.Icon,
			func(tx *sql.Tx) error {
				return s.links.SyncLinksFromHTML(tx, docID, docHTML)
			},
		)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrVersionConflict) {
			stored, _ := s.docs.GetByID(id)
			writeJSON(w, http.StatusConflict, model.VersionConflict{
				StoredVersion: stored.Version,
				Stored:        stored,
			})
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

func (s *Server) handleDeleteDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := s.docs.SoftDelete(id); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleMoveDocument() http.HandlerFunc {
	type request struct {
		NewParentID *int64 `json:"new_parent_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.docs.Move(id, req.NewParentID); errors.Is(err, store.ErrCircularMove) {
			http.Error(w, "circular move not allowed", http.StatusBadRequest)
			return
		} else if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleListChildren() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		children, err := s.docs.ListChildren(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if children == nil {
			children = []*model.Document{}
		}
		writeJSON(w, http.StatusOK, children)
	}
}

func (s *Server) handleRestoreDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := s.docs.Restore(id); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
