package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

func (s *Server) handleCreateShare() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		// Verify document exists
		if _, err := s.docs.GetByID(docID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		plaintext, share, err := s.shares.Create(docID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Build the share URL. In production this is the external URL; in
		// development it is the server's own address.
		shareURL := fmt.Sprintf("/public/%s", plaintext)

		writeJSON(w, http.StatusCreated, model.ShareCreateResponse{
			Token:    plaintext,
			URL:      shareURL,
			RevokeID: share.ID,
		})
	}
}

func (s *Server) handleRevokeShare() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shareID, err := parseID(r, "shareID")
		if err != nil {
			http.Error(w, "invalid share id", http.StatusBadRequest)
			return
		}
		if err := s.shares.Revoke(shareID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handlePublicShare() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if token == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		shareLink, err := s.shares.LookupByToken(token)
		if errors.Is(err, store.ErrNotFound) {
			// Always 404 — never 401 — to avoid leaking token existence
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		doc, err := s.docs.GetByID(shareLink.DocumentID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Sanitize with the stricter public-share policy before rendering
		safeBody := security.SanitizePublicHTML(doc.BodyHTML)

		// Render share.html with the document content injected server-side.
		// We reuse the embedded share.html as a template placeholder.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s — PKD</title>
  <link rel="stylesheet" href="/css/app.css">
</head>
<body class="share-view">
  <article id="share-content">
    <h1 id="share-title">%s</h1>
    <div id="share-body" class="ck-content">%s</div>
  </article>
</body>
</html>`, htmlEscape(doc.Title), htmlEscape(doc.Title), safeBody)
	}
}

func htmlEscape(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			b = append(b, []byte("&lt;")...)
		case '>':
			b = append(b, []byte("&gt;")...)
		case '&':
			b = append(b, []byte("&amp;")...)
		case '"':
			b = append(b, []byte("&#34;")...)
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}
