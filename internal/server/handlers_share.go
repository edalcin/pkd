package server

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// sharePageData holds the values rendered into sharePageTmpl.
type sharePageData struct {
	Title string
	Icon  string
	Tags  []string
	Date  string
	Body  template.HTML // pre-sanitized by SanitizePublicHTML — safe to skip escaping
}

var sharePageTmpl = template.Must(template.New("share").Parse(sharePageHTML))

func (s *Server) handleCreateShare() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if _, err := s.docs.GetByID(docID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		plaintext, share, err := s.shares.Create(docID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

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

var ptMonths = [...]string{
	"", "jan.", "fev.", "mar.", "abr.", "mai.", "jun.",
	"jul.", "ago.", "set.", "out.", "nov.", "dez.",
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

		icon := doc.Icon
		if icon == "" {
			icon = "📄"
		}
		date := fmt.Sprintf("%d de %s de %d",
			doc.CreatedAt.Day(), ptMonths[doc.CreatedAt.Month()], doc.CreatedAt.Year())

		data := sharePageData{
			Title: doc.Title,
			Icon:  icon,
			Tags:  doc.Tags,
			Date:  date,
			Body:  template.HTML(security.SanitizePublicHTML(doc.BodyHTML)),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = sharePageTmpl.Execute(w, data)
	}
}

const sharePageHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} — PKD</title>
  <style>
    :root {
      --bg:      #f8f7f4;
      --surface: #fff;
      --text:    #1c1917;
      --muted:   #78716c;
      --border:  #e7e5e4;
      --accent:  #6366f1;
      --radius:  10px;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg:      #1c1917;
        --surface: #292524;
        --text:    #e7e5e4;
        --muted:   #a8a29e;
        --border:  #3d3835;
        --accent:  #818cf8;
      }
    }
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif;
      font-size: 15px;
      background: var(--bg);
      color: var(--text);
      line-height: 1.6;
      padding: 24px 16px 48px;
    }
    .page { max-width: 720px; margin: 0 auto; }

    /* ── Page header ─────────────────────────── */
    .page-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 24px;
      padding-bottom: 12px;
      border-bottom: 1px solid var(--border);
    }
    .page-brand {
      font-size: 14px;
      color: var(--muted);
      text-decoration: none;
    }
    .page-brand:hover { color: var(--accent); }
    .note-date { font-size: 13px; color: var(--muted); }

    /* ── Note card ───────────────────────────── */
    .note-card {
      background: var(--surface);
      border-radius: var(--radius);
      border: 1px solid var(--border);
      padding: 28px 32px;
    }
    .note-header {
      padding-bottom: 18px;
      margin-bottom: 18px;
      border-bottom: 1px solid var(--border);
    }
    .note-meta {
      display: flex;
      align-items: flex-start;
      gap: 10px;
    }
    .note-icon { font-size: 1.4em; line-height: 1.25; flex-shrink: 0; }
    .note-title {
      font-size: 1.5em;
      font-weight: 700;
      line-height: 1.25;
      letter-spacing: -.02em;
    }
    .note-tags { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 10px; }
    .tag {
      font-size: 12px;
      color: var(--muted);
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 4px;
      padding: 1px 7px;
    }

    /* ── Prose ───────────────────────────────── */
    .prose h1, .prose h2, .prose h3,
    .prose h4, .prose h5, .prose h6 { margin: 0.75em 0 0.25em; font-weight: 700; }
    .prose h1 { font-size: 1.35em; }
    .prose h2 { font-size: 1.2em; }
    .prose h3 { font-size: 1.05em; }
    .prose p { margin: 0.5em 0; }
    .prose a { color: var(--accent); }
    .prose strong { font-weight: 600; }
    .prose ul, .prose ol { padding-left: 1.5em; margin: 0.5em 0; }
    .prose li { margin: 0.25em 0; }
    .prose li::marker { color: var(--muted); }
    .prose hr { border: none; border-top: 1px solid var(--border); margin: 1.5em 0; }
    .prose blockquote {
      border-left: 3px solid var(--border);
      margin: 0.75em 0;
      padding: 0 0.75em;
      color: var(--muted);
      font-style: italic;
    }
    .prose blockquote p { margin: 0; }
    .prose code {
      font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
      font-size: 0.875em;
    }
    .prose p code, .prose li code {
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 4px;
      padding: 1px 5px;
    }
    .prose pre {
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 14px 16px;
      overflow-x: auto;
      font-size: 13px;
      margin: 0.75em 0;
      line-height: 1.55;
    }
    .prose pre code { background: none; border: none; padding: 0; font-size: 1em; }
    .prose img { max-width: 100%; border-radius: 6px; margin: 0.5em 0; }
    .prose figure { margin: 0.75em 0; }
    .prose figcaption { font-size: 0.85em; color: var(--muted); text-align: center; margin-top: 4px; }
    .prose table { width: 100%; border-collapse: collapse; font-size: 0.9em; margin: 0.75em 0; }
    .prose th {
      border-bottom: 1px solid var(--border);
      padding: 6px 10px;
      text-align: left;
      font-size: 0.8em;
      font-weight: 600;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: .04em;
    }
    .prose td { border-bottom: 1px solid var(--border); padding: 6px 10px; vertical-align: top; }
    .prose tbody tr:last-child td { border-bottom: none; }

    @media (max-width: 600px) {
      .note-card { padding: 20px 18px; }
      .note-title { font-size: 1.25em; }
    }
  </style>
</head>
<body>
  <div class="page">

    <header class="page-header">
      <a href="/" class="page-brand">📄 PKD</a>
      <time class="note-date">{{.Date}}</time>
    </header>

    <article class="note-card">
      <header class="note-header">
        <div class="note-meta">
          <span class="note-icon">{{.Icon}}</span>
          <h1 class="note-title">{{.Title}}</h1>
        </div>
        {{if .Tags}}
        <div class="note-tags">
          {{range .Tags}}<span class="tag">#{{.}}</span>{{end}}
        </div>
        {{end}}
      </header>
      <div class="prose">{{.Body}}</div>
    </article>

  </div>
</body>
</html>`
