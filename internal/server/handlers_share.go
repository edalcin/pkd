package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// shareChildData holds display info for a child document on the public share page.
type shareChildData struct {
	Title        string
	Icon         string
	IconIsBoxicon bool
	URL          string
}

// sharePageData holds the values rendered into sharePageTmpl.
type sharePageData struct {
	Title        string
	Icon         string
	IconIsBoxicon bool
	Tags         []string
	Date         string
	Body         template.HTML // pre-sanitized by SanitizePublicHTML — safe to skip escaping
	Children     []shareChildData
	ParentTitle  string
	ParentURL    string
}

var sharePageTmpl = template.Must(template.New("share").Parse(sharePageHTML))

// collectDescendantIDs returns all descendant document IDs of rootID (BFS, non-recursive).
func (s *Server) collectDescendantIDs(rootID int64) []int64 {
	var ids []int64
	queue := []int64{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		children, err := s.docs.ListChildren(cur)
		if err != nil {
			break
		}
		for _, c := range children {
			ids = append(ids, c.ID)
			queue = append(queue, c.ID)
		}
	}
	return ids
}

func (s *Server) handleCreateShare() http.HandlerFunc {
	type createShareRequest struct {
		IncludeChildren *bool `json:"include_children"`
		IncludeParent   *bool `json:"include_parent"`
	}
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

		var req createShareRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}
		// Default: include children (backward-compatible); do NOT include parent by default.
		includeChildren := true
		if req.IncludeChildren != nil {
			includeChildren = *req.IncludeChildren
		}
		includeParent := false
		if req.IncludeParent != nil {
			includeParent = *req.IncludeParent
		}

		plaintext, share, err := s.shares.Create(docID, includeChildren, includeParent)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Auto-create shares for all descendants only when recursive.
		if includeChildren {
			for _, id := range s.collectDescendantIDs(docID) {
				_ = s.shares.CreateAuto(id)
			}
		}

		base := s.baseURL(r)
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
		docID, err := s.shares.Revoke(shareID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Cascade: revoke auto shares for all descendants.
		for _, id := range s.collectDescendantIDs(docID) {
			_ = s.shares.RevokeAutoForDocument(id)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// baseURL returns the base URL string (with trailing slash) to use for share links.
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/"
}

var ptMonths = [...]string{
	"", "jan.", "fev.", "mar.", "abr.", "mai.", "jun.",
	"jul.", "ago.", "set.", "out.", "nov.", "dez.",
}

func isBoxicon(icon string) bool {
	return strings.HasPrefix(icon, "bx")
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

		base := s.baseURL(r)

		// Fetch children only if the share was created with include_children=true.
		var childData []shareChildData
		if shareLink.IncludeChildren {
			children, err := s.docs.ListChildren(doc.ID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			for _, child := range children {
				childToken, cerr := s.shares.GetActiveShareForDocument(child.ID)
				if cerr != nil || childToken == "" {
					continue // only list children that already have an explicit share
				}
				childIcon := child.Icon
				if childIcon == "" {
					childIcon = "📄"
				}
				childData = append(childData, shareChildData{
					Title:         child.Title,
					Icon:          childIcon,
					IconIsBoxicon: isBoxicon(childIcon),
					URL:           base + "public/" + childToken,
				})
			}
		}

		// Resolve parent link only if this share was created with include_parent=true
		// AND the parent already has an active share.
		var parentTitle, parentURL string
		if shareLink.IncludeParent && doc.ParentID != nil {
			if parent, perr := s.docs.GetByID(*doc.ParentID); perr == nil {
				if tok, perr := s.shares.GetActiveShareForDocument(parent.ID); perr == nil && tok != "" {
					parentTitle = parent.Title
					parentURL = base + "public/" + tok
				}
			}
		}

		// Rewrite attachment src to the token-scoped public route so images
		// load without requiring a session (the authenticated /api/attachments/
		// route 401s for anonymous visitors).
		publicBodyHTML := strings.ReplaceAll(doc.BodyHTML, `src="/api/attachments/`, `src="/public/`+token+`/attachments/`)

		data := sharePageData{
			Title:        doc.Title,
			Icon:         icon,
			IconIsBoxicon: isBoxicon(icon),
			Tags:         doc.Tags,
			Date:         date,
			Body:         template.HTML(security.SanitizePublicHTML(publicBodyHTML)),
			Children:     childData,
			ParentTitle:  parentTitle,
			ParentURL:    parentURL,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = sharePageTmpl.Execute(w, data)
	}
}

// handlePublicAttachment serves an attachment referenced by a publicly shared
// document's body, without requiring an authenticated session. Access is
// scoped tightly: the attachment must belong to the exact document the share
// token points at (share bodies never embed a child document's images
// inline, so no broader check is needed).
func (s *Server) handlePublicAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		shareLink, err := s.shares.LookupByToken(token)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		att, err := s.attachments.GetByID(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if att.DocumentID != shareLink.DocumentID {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		s.serveAttachmentFile(w, r, att)
	}
}

const sharePageHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} — PKD</title>
  <link rel="stylesheet" href="https://unpkg.com/boxicons@2.1.4/css/boxicons.min.css">
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
      overflow-x: hidden;
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
    .prose {
      overflow-wrap: break-word;
      word-break: break-word;
    }
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
      white-space: pre-wrap;
      word-break: break-all;
      font-size: 13px;
      margin: 0.75em 0;
      line-height: 1.55;
      max-width: 100%;
    }
    .prose pre code { background: none; border: none; padding: 0; font-size: 1em; white-space: pre; }
    .prose img { max-width: 100%; height: auto; border-radius: 6px; margin: 0.5em 0; display: block; }
    .prose figure { margin: 0.75em 0; }
    .prose figcaption { font-size: 0.85em; color: var(--muted); text-align: center; margin-top: 4px; }
    .prose iframe, .prose video, .prose embed { max-width: 100%; }
    .prose table { display: block; overflow-x: auto; border-collapse: collapse; font-size: 0.9em; max-width: 100%; }
    .prose th {
      border-bottom: 1px solid var(--border);
      padding: 6px 10px;
      text-align: left;
      font-size: 0.8em;
      font-weight: 600;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: .04em;
      white-space: nowrap;
    }
    .prose td { border-bottom: 1px solid var(--border); padding: 6px 10px; vertical-align: top; }
    .prose tbody tr:last-child td { border-bottom: none; }

    /* ── Parent breadcrumb ───────────────────── */
    .parent-back {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-size: 13px;
      color: var(--muted);
      text-decoration: none;
      margin-bottom: 16px;
      transition: color 0.15s;
    }
    .parent-back:hover { color: var(--accent); }
    .parent-back .bx { font-size: 1.1em; }

    /* ── Children list ───────────────────────── */
    .children-section {
      margin-top: 20px;
      padding-top: 18px;
      border-top: 1px solid var(--border);
    }
    .children-list {
      list-style: none;
      display: flex;
      flex-direction: column;
      gap: 4px;
    }
    .children-list li a {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 7px 10px;
      border-radius: 6px;
      text-decoration: none;
      color: var(--text);
      font-size: 14px;
      transition: background 0.15s;
    }
    .children-list li a:hover { background: var(--bg); color: var(--accent); }
    .child-icon { font-size: 1em; color: var(--muted); flex-shrink: 0; }

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

    {{if .ParentURL}}
    <a href="{{.ParentURL}}" class="parent-back">
      <i class="bx bx-arrow-back"></i> {{.ParentTitle}}
    </a>
    {{end}}

    <article class="note-card">
      <header class="note-header">
        <div class="note-meta">
          {{if .IconIsBoxicon}}<i class="bx {{.Icon}} note-icon"></i>{{else}}<span class="note-icon">{{.Icon}}</span>{{end}}
          <h1 class="note-title">{{.Title}}</h1>
        </div>
        {{if .Tags}}
        <div class="note-tags">
          {{range .Tags}}<span class="tag">#{{.}}</span>{{end}}
        </div>
        {{end}}
      </header>
      <div class="prose">{{.Body}}</div>
      {{if .Children}}
      <div class="children-section">
        <ul class="children-list">
          {{range .Children}}
          <li>
            <a href="{{.URL}}">
              {{if .IconIsBoxicon}}<i class="bx {{.Icon}} child-icon"></i>{{else}}<span class="child-icon">{{.Icon}}</span>{{end}}
              {{.Title}}
            </a>
          </li>
          {{end}}
        </ul>
      </div>
      {{end}}
    </article>

  </div>
</body>
</html>`
