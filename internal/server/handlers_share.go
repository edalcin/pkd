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
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}} — PKD</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Playfair+Display:ital,wght@0,600;0,700;1,400&family=Crimson+Pro:ital,opsz,wght@0,6..72,300;0,6..72,400;1,6..72,300;1,6..72,400&family=JetBrains+Mono:wght@400&display=swap" rel="stylesheet">
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    :root {
      --bg:         #FAF7F0;
      --surface:    #FFFFFF;
      --text:       #1C1917;
      --text-muted: #78716C;
      --text-faint: #A8A29E;
      --accent:     #B45309;
      --accent-mid: #D97706;
      --accent-lt:  #FEF3C7;
      --accent-bd:  #FDE68A;
      --border:     #E7E5E4;
      --code-bg:    #F5F0E8;
      --code-bd:    #EDE9DF;
      --pre-bg:     #1C1917;
      --pre-text:   #E7E5E4;
      --quote-bg:   #FDFAF5;
    }

    ::selection { background: #FDE68A; color: #78350F; }

    html { font-size: 18px; scroll-behavior: smooth; }

    body {
      font-family: 'Crimson Pro', Georgia, 'Times New Roman', serif;
      background-color: var(--bg);
      background-image:
        radial-gradient(at 12% 18%, rgba(251,191,36,.13) 0, transparent 52%),
        radial-gradient(at 88% 82%, rgba(234,88,12,.09) 0, transparent 52%);
      min-height: 100vh;
      color: var(--text);
      line-height: 1.75;
      padding: 3rem 1.25rem 5rem;
    }

    /* ── Container ───────────────────────────── */
    .wrap { max-width: 740px; margin: 0 auto; }

    /* ── Card ────────────────────────────────── */
    .card {
      background: var(--surface);
      border-radius: 4px;
      padding: 3.5rem 4rem;
      border-top: 3px solid var(--accent-mid);
      box-shadow:
        0 0 0 1px rgba(0,0,0,.03),
        0 4px 16px rgba(0,0,0,.05),
        0 24px 64px rgba(0,0,0,.07);
      animation: rise .55s cubic-bezier(.22,1,.36,1) both;
    }

    @keyframes rise {
      from { opacity: 0; transform: translateY(22px); }
      to   { opacity: 1; transform: translateY(0);    }
    }

    /* ── Header ──────────────────────────────── */
    .doc-header { margin-bottom: 2.75rem; }

    .eyebrow {
      display: flex;
      align-items: center;
      gap: .75rem;
      margin-bottom: 1.25rem;
    }

    .doc-icon { font-size: 1.75rem; line-height: 1; flex-shrink: 0; }

    .doc-date {
      font-family: 'Crimson Pro', Georgia, serif;
      font-style: italic;
      font-size: .85rem;
      color: var(--text-faint);
      letter-spacing: .03em;
    }

    .doc-title {
      font-family: 'Playfair Display', Georgia, serif;
      font-size: clamp(1.85rem, 4.5vw, 2.9rem);
      font-weight: 700;
      line-height: 1.18;
      letter-spacing: -.02em;
      color: var(--text);
      margin-bottom: 1.4rem;
    }

    .title-rule {
      width: 42px;
      height: 3px;
      background: linear-gradient(90deg, var(--accent), var(--accent-mid));
      border-radius: 2px;
      margin-bottom: 1.25rem;
    }

    .doc-tags { display: flex; flex-wrap: wrap; gap: .35rem; }

    .tag {
      font-family: 'Crimson Pro', Georgia, serif;
      font-style: italic;
      font-size: .78rem;
      color: var(--accent);
      background: var(--accent-lt);
      border: 1px solid var(--accent-bd);
      border-radius: 20px;
      padding: .18em .65em;
      letter-spacing: .02em;
    }

    /* ── Prose ───────────────────────────────── */
    .prose {
      font-size: 1rem;
      line-height: 1.8;
      color: #292524;
      font-weight: 300;
    }

    .prose > * + * { margin-top: 1.2rem; }

    .prose h1, .prose h2, .prose h3,
    .prose h4, .prose h5, .prose h6 {
      font-family: 'Playfair Display', Georgia, serif;
      color: var(--text);
      line-height: 1.25;
      letter-spacing: -.015em;
      margin-top: 2.25rem;
      margin-bottom: .4rem;
    }
    .prose h1 { font-size: 1.85rem; font-weight: 700; }
    .prose h2 { font-size: 1.45rem; font-weight: 700; }
    .prose h3 { font-size: 1.2rem;  font-weight: 600; }
    .prose h4 { font-size: 1.05rem; font-weight: 600; font-style: italic; }

    .prose p { margin-top: 1.1rem; }

    .prose a {
      color: var(--accent);
      text-decoration: underline;
      text-decoration-thickness: 1px;
      text-underline-offset: 3px;
    }
    .prose a:hover { color: #92400E; }

    .prose strong { font-weight: 600; color: var(--text); }

    .prose ul, .prose ol { padding-left: 1.5rem; margin-top: 1.1rem; }
    .prose li { margin-top: .35rem; }
    .prose li::marker { color: var(--accent-mid); }

    .prose hr {
      border: none;
      border-top: 1px solid var(--border);
      margin: 2.5rem 0;
    }

    .prose blockquote {
      position: relative;
      margin: 1.75rem 0;
      padding: 1rem 1.5rem 1rem 2.75rem;
      background: var(--quote-bg);
      border-left: 3px solid var(--accent-mid);
      border-radius: 0 4px 4px 0;
    }
    .prose blockquote::before {
      content: '\201C';
      font-family: 'Playfair Display', Georgia, serif;
      font-size: 4rem;
      color: var(--accent-mid);
      position: absolute;
      left: .45rem;
      top: -.6rem;
      line-height: 1;
      opacity: .35;
    }
    .prose blockquote p { margin-top: 0; font-style: italic; color: #44403C; font-size: 1.05em; }

    .prose code {
      font-family: 'JetBrains Mono', 'Courier New', monospace;
      font-size: .8em;
      background: var(--code-bg);
      color: var(--accent);
      padding: .15em .4em;
      border-radius: 3px;
      border: 1px solid var(--code-bd);
    }

    .prose pre {
      background: var(--pre-bg);
      color: var(--pre-text);
      padding: 1.4rem 1.6rem;
      border-radius: 6px;
      overflow-x: auto;
      margin: 1.75rem 0;
      font-size: .82em;
      line-height: 1.65;
      box-shadow: inset 0 1px 0 rgba(255,255,255,.04);
    }
    .prose pre code {
      background: none; color: inherit;
      padding: 0; border: none; font-size: 1em;
    }

    .prose img {
      max-width: 100%; height: auto;
      border-radius: 6px;
      margin: 1.5rem 0;
      box-shadow: 0 2px 14px rgba(0,0,0,.1);
    }

    .prose figure { margin: 1.75rem 0; }
    .prose figcaption {
      margin-top: .5rem;
      font-size: .82em;
      font-style: italic;
      color: var(--text-faint);
      text-align: center;
    }

    .prose table {
      width: 100%;
      border-collapse: collapse;
      margin: 1.75rem 0;
      font-size: .9em;
    }
    .prose thead tr { border-bottom: 2px solid var(--border); }
    .prose th {
      padding: .55rem 1rem;
      text-align: left;
      font-family: 'Playfair Display', Georgia, serif;
      font-weight: 600;
      font-size: .88em;
      color: #44403C;
      letter-spacing: .02em;
    }
    .prose td {
      padding: .55rem 1rem;
      border-bottom: 1px solid #F5F0E8;
      vertical-align: top;
    }
    .prose tbody tr:last-child td { border-bottom: none; }
    .prose tbody tr:hover td { background: #FDFAF5; }

    /* ── Footer ──────────────────────────────── */
    .doc-footer {
      margin-top: 4rem;
      padding-top: 2rem;
      border-top: 1px solid var(--border);
      display: flex;
      align-items: center;
      justify-content: center;
      gap: .75rem;
    }

    .footer-ornament {
      color: var(--accent-mid);
      font-size: 1rem;
      opacity: .45;
      letter-spacing: .4em;
    }

    .footer-brand {
      font-family: 'Crimson Pro', Georgia, serif;
      font-style: italic;
      font-size: .8rem;
      color: var(--text-faint);
    }
    .footer-brand strong {
      font-style: normal;
      font-weight: 600;
      color: var(--text-muted);
      letter-spacing: .04em;
    }

    /* ── Responsive ──────────────────────────── */
    @media (max-width: 600px) {
      body  { padding: 1rem .75rem 3rem; }
      .card { padding: 2rem 1.5rem; }
      .doc-title { font-size: 1.75rem; }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <article class="card">

      <header class="doc-header">
        <div class="eyebrow">
          <span class="doc-icon">{{.Icon}}</span>
          <time class="doc-date">{{.Date}}</time>
        </div>
        <h1 class="doc-title">{{.Title}}</h1>
        <div class="title-rule"></div>
        {{if .Tags}}
        <div class="doc-tags">
          {{range .Tags}}<span class="tag">#{{.}}</span>{{end}}
        </div>
        {{end}}
      </header>

      <div class="prose">{{.Body}}</div>

      <footer class="doc-footer">
        <span class="footer-ornament">✦ ✦ ✦</span>
        <p class="footer-brand">Compartilhado via <strong>PKD</strong></p>
      </footer>

    </article>
  </div>
</body>
</html>`
