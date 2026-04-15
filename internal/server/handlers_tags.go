package server

import (
	"encoding/json"
	"net/http"

	"github.com/edalcin/pkd/internal/model"
)

func (s *Server) handleListTags() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tags, err := s.tags.ListWithCounts()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if tags == nil {
			tags = []*model.TagWithCount{}
		}
		writeJSON(w, http.StatusOK, tags)
	}
}

func (s *Server) handleSetDocumentTags() http.HandlerFunc {
	type request struct {
		Tags []string `json:"tags"`
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
		if err := s.tags.SetDocumentTags(id, req.Tags); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
