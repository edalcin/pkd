package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/pkd/internal/store"
)

// handleListLinks serves GET /api/documents/{id}/links.
// Returns outgoing links (forward) and incoming backlinks for the document.
func (s *Server) handleListLinks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		resp, err := s.links.GetLinksForDocument(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleCreateLink serves POST /api/documents/{id}/links.
// Body: {"target_id": 42}
func (s *Server) handleCreateLink() http.HandlerFunc {
	type request struct {
		TargetID int64 `json:"target_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sourceID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetID == 0 {
			http.Error(w, "target_id required", http.StatusBadRequest)
			return
		}
		entry, err := s.links.CreateLink(sourceID, req.TargetID)
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "link already exists", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, entry)
	}
}

// handleDeleteLink serves DELETE /api/documents/{id}/links/{linkId}.
func (s *Server) handleDeleteLink() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		linkID, err := parseID(r, "linkId")
		if err != nil {
			http.Error(w, "invalid linkId", http.StatusBadRequest)
			return
		}
		if err := s.links.DeleteLink(linkID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
