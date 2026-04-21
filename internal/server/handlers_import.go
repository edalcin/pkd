package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

// handleImport serves POST /api/import.
// Authenticated via Bearer token (PKD_IMPORT_TOKEN env var).
// Creates a document from the provided content and applies the #notas tag
// plus any extra tags supplied by the caller.
func (s *Server) handleImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title   string   `json:"title"`
			Content string   `json:"content"`
			Tags    []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		title := strings.TrimSpace(body.Title)
		if title == "" {
			title = "Nota " + time.Now().Format("2006-01-02 15:04")
		}

		safeHTML := security.SanitizeEditorHTML(body.Content)
		plainText := security.ExtractPlainText(safeHTML)

		doc, err := s.docs.Create(nil, title)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		doc, err = s.docs.Update(doc.ID, doc.Version, title, safeHTML, plainText, "")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Always tag with "notas"; append caller-supplied tags (note hashtags)
		allTags := append([]string{"notas"}, body.Tags...)
		if err := s.tags.SetDocumentTags(doc.ID, allTags); err != nil {
			_ = err // non-fatal — document is still created
		}

		doc, err = s.docs.GetByID(doc.ID)
		if err != nil {
			writeJSON(w, http.StatusCreated, doc)
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	}
}

// ImportTokenAuth returns a middleware that validates a static Bearer token.
func ImportTokenAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
