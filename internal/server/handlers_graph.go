package server

import (
	"encoding/json"
	"net/http"
)

// handleGraph serves GET /api/graph.
// Returns graph nodes and edges for D3.js visualisation.
//
// Query parameters:
//
//	tag=<name>   Filter by tag (repeatable, AND semantics)
//	all=true     Include isolated documents (no links) in the result
func (s *Server) handleGraph() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tags := r.URL.Query()["tag"]
		includeAll := r.URL.Query().Get("all") == "true"

		data, err := s.links.GetGraphData(tags, includeAll)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, data)
	}
}

// handleSemanticGraph serves GET /api/graph/semantic.
// Returns semantic edges (Gemini embeddings, cached in SQLite).
func (s *Server) handleSemanticGraph() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.GeminiAPIKey == "" {
			http.Error(w, `{"error":"GEMINI_API_KEY not configured"}`, http.StatusServiceUnavailable)
			return
		}
		edges, err := s.links.GetSemanticEdges(r.Context(), s.cfg.GeminiAPIKey)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"edges": edges})
	}
}

// handleCommunityName serves POST /api/community/name.
// Body: {"titles":["Doc A","Doc B",...]}  Response: {"name":"..."}
func (s *Server) handleCommunityName() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.GeminiAPIKey == "" {
			http.Error(w, `{"error":"GEMINI_API_KEY not configured"}`, http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Titles []string `json:"titles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Titles) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		chatModel, _ := s.settings.ChatModel()
		name, err := s.links.SuggestCommunityName(r.Context(), s.cfg.GeminiAPIKey, chatModel, req.Titles)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": name})
	}
}
