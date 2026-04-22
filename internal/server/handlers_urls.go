package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/pkd/internal/store"
)

func (s *Server) handleListURLs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		urls, err := s.urls.ListByDocument(docID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, urls)
	}
}

func (s *Server) handleCreateURL() http.HandlerFunc {
	type request struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		u, err := s.urls.Create(docID, req.URL, req.Title)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, u)
	}
}

func (s *Server) handleUpdateURL() http.HandlerFunc {
	type request struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		urlID, err := parseID(r, "urlId")
		if err != nil {
			http.Error(w, "invalid urlId", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		u, err := s.urls.Update(urlID, req.URL, req.Title)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

func (s *Server) handleDeleteURL() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		urlID, err := parseID(r, "urlId")
		if err != nil {
			http.Error(w, "invalid urlId", http.StatusBadRequest)
			return
		}
		if err := s.urls.Delete(urlID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
