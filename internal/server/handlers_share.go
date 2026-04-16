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

		// Build the fully-qualified share URL.
		// Prefer PKD_BASE_URL (e.g. "https://pkd.dalc.in/") so links are correct
		// behind a reverse proxy. Fall back to deriving the base from the request.
		var base string
		if s.cfg.BaseURL != "" {
			base = s.cfg.BaseURL
		} else {
			scheme := "http"
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			base = scheme + "://" + r.Host + "/"
		}
		shareURL := base + "public/" + plaintext

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

		// Render a minimal read-only HTML page. No JS runs (CSP: script-src 'none').
		// Styles are inlined to avoid an extra round-trip and stay self-contained.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s — PKD</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0 }
    body { font-family: system-ui, -apple-system, sans-serif; background: #f8f8f8;
           color: #1a1a1a; line-height: 1.7; padding: 2rem 1rem }
    article { max-width: 720px; margin: 0 auto; background: #fff;
              border-radius: 10px; padding: 2rem 2.5rem;
              box-shadow: 0 1px 4px rgba(0,0,0,.08) }
    h1 { font-size: 1.75rem; font-weight: 700; margin-bottom: 1.25rem;
         border-bottom: 1px solid #e5e5e5; padding-bottom: .75rem }
    h2 { font-size: 1.3rem; margin-top: 1.5rem } h3 { font-size: 1.1rem; margin-top: 1.25rem }
    p { margin-top: .75rem } ul, ol { padding-left: 1.5rem; margin-top: .75rem }
    code { background: #f0f0f0; padding: .1em .3em; border-radius: 3px; font-size: .875em }
    pre { background: #f0f0f0; padding: .75em; border-radius: 6px; overflow-x: auto; margin-top: .75rem }
    pre code { background: none; padding: 0 }
    img { max-width: 100%%; border-radius: 6px; margin-top: .75rem }
    blockquote { border-left: 3px solid #4f46e5; padding-left: 1em;
                 color: #6b7280; margin-top: .75rem }
    a { color: #4f46e5 }
    table { border-collapse: collapse; width: 100%%; margin-top: .75rem }
    th, td { border: 1px solid #e5e5e5; padding: .4rem .75rem; text-align: left }
    th { background: #f8f8f8 }
    .pkd-badge { margin-top: 2rem; font-size: .8rem; color: #9ca3af; text-align: center }
  </style>
</head>
<body>
  <article>
    <h1>%s</h1>
    <div>%s</div>
    <p class="pkd-badge">Compartilhado via <strong>PKD</strong></p>
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
