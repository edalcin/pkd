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
		if errors.Is(err, store.ErrLocked) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "document is locked"})
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

func (s *Server) handleToggleLock() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.ToggleLock(id)
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

func (s *Server) handleUpdateAssocDate() http.HandlerFunc {
	type request struct {
		Year  *int `json:"year"`
		Month *int `json:"month"`
		Day   *int `json:"day"`
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

		// Validate partial date combinations (FR-004, FR-005)
		if req.Day != nil && req.Month == nil {
			http.Error(w, "day requires month", http.StatusBadRequest)
			return
		}
		if req.Month != nil && req.Year == nil {
			http.Error(w, "month requires year", http.StatusBadRequest)
			return
		}
		if req.Month != nil && (*req.Month < 1 || *req.Month > 12) {
			http.Error(w, "month out of range", http.StatusBadRequest)
			return
		}
		if req.Day != nil && (*req.Day < 1 || *req.Day > 31) {
			http.Error(w, "day out of range", http.StatusBadRequest)
			return
		}

		doc, err := s.docs.UpdateAssocDate(id, req.Year, req.Month, req.Day)
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

func (s *Server) handleDeleteDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		err = s.docs.SoftDelete(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrLocked) {
			http.Error(w, "locked", http.StatusForbidden)
			return
		}
		if err != nil {
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

func (s *Server) handleReorderDocument() http.HandlerFunc {
	type request struct {
		NewParentID *int64 `json:"parent_id"`
		BeforeID    *int64 `json:"before_id"`
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
		if err := s.docs.Reorder(id, req.NewParentID, req.BeforeID); errors.Is(err, store.ErrCircularMove) {
			http.Error(w, "circular move not allowed", http.StatusBadRequest)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleSortTree() http.HandlerFunc {
	type request struct {
		By string `json:"by"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.By == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.docs.SortAll(req.By); err != nil {
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

func (s *Server) handleListAncestors() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		ancestors, err := s.docs.Ancestors(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if ancestors == nil {
			ancestors = []*model.Ancestor{}
		}
		writeJSON(w, http.StatusOK, ancestors)
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

func (s *Server) handleToggleFavorite() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.ToggleFavorite(id)
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

func (s *Server) handleArchiveDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.Archive(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrArchived) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "document is already archived"})
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

func (s *Server) handleUnarchiveDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.Unarchive(id)
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

func (s *Server) handleListVersions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		versions, err := s.docs.ListVersions(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if versions == nil {
			versions = []model.DocumentVersion{}
		}
		writeJSON(w, http.StatusOK, versions)
	}
}

func (s *Server) handleGetVersion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		vid, err := parseID(r, "vid")
		if err != nil {
			http.Error(w, "invalid version id", http.StatusBadRequest)
			return
		}
		v, err := s.docs.GetVersion(id, vid)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (s *Server) handleRestoreVersion() http.HandlerFunc {
	type request struct {
		Version int64 `json:"version"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		vid, err := parseID(r, "vid")
		if err != nil {
			http.Error(w, "invalid version id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		snap, err := s.docs.GetVersion(id, vid)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Re-derive plain text for FTS from the snapshotted HTML.
		// The HTML is already sanitized; we only need plain text for search indexing.
		plainText := security.ExtractPlainText(snap.BodyHTML)

		docID := id
		docHTML := snap.BodyHTML
		doc, err := s.docs.UpdateAndSync(id, req.Version, snap.Title, snap.BodyHTML, plainText, snap.Icon,
			func(tx *sql.Tx) error {
				return s.links.SyncLinksFromHTML(tx, docID, docHTML)
			},
		)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrLocked) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "document is locked"})
			return
		}
		if errors.Is(err, store.ErrArchived) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "document is archived"})
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

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
