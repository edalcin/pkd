package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
)

// handleCapture serves POST /api/capture.
// Accepts both application/json and application/x-www-form-urlencoded (PWA share_target).
// Creates a Nota (same store path as POST /api/notes) from the captured
// content and tags it with the request's tags, or #captura when there are
// none. An optional idempotency_key follows the
// same replay semantics as /api/notes: the same key returns 200 with the
// existing Nota instead of creating a duplicate.
func (s *Server) handleCapture() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// share_target form posts are a page navigation: send the browser to
		// the new Nota instead of showing raw JSON.
		shareTarget := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
		respond := func(status int, doc *model.Document) {
			if shareTarget && doc != nil {
				http.Redirect(w, r, fmt.Sprintf("/#/doc/%d", doc.ID), http.StatusSeeOther)
				return
			}
			writeJSON(w, status, doc)
		}
		title, content, rawURL, extraTags, idempotencyKey := parseCaptureBody(r)
		idempotencyKey = strings.TrimSpace(idempotencyKey)
		if len(idempotencyKey) > maxIdempotencyKeyLen {
			http.Error(w, "idempotency_key too long", http.StatusBadRequest)
			return
		}

		// If a URL was provided, attempt Open Graph extraction (best-effort)
		if rawURL != "" {
			og := fetchOpenGraph(rawURL)
			if og.title != "" && title == "" {
				title = og.title
			}
			if og.description != "" && content == "" {
				content = og.description
			}
			// Prepend the URL as the first line of body if not already in content
			if !strings.Contains(content, rawURL) {
				content = fmt.Sprintf("<p><a href=%q>%s</a></p>\n%s", rawURL, rawURL, content)
			}
		}

		if title == "" {
			title = "Captura " + time.Now().Format("2006-01-02 15:04")
		}

		// Sanitize content
		safeHTML := security.SanitizeEditorHTML(content)
		plainText := security.ExtractPlainText(safeHTML)

		// Create the Nota (idempotencyKey replay returns the existing one)
		doc, created, err := s.docs.CreateNote(title, idempotencyKey, false, nil, nil)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !created {
			respond(http.StatusOK, doc)
			return
		}

		// Save body
		doc, err = s.docs.Update(doc.ID, doc.Version, title, safeHTML, plainText, "")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Apply tags: the request's tags replace the #captura default
		// (pkdMobile sends ["android"]; the PWA form sends none).
		allTags := extraTags
		if len(allTags) == 0 {
			allTags = []string{"captura"}
		}
		if err := s.tags.SetDocumentTags(doc.ID, allTags); err != nil {
			// Non-fatal — Nota is still created
			_ = err
		}

		// Re-fetch with tags included
		if fresh, err := s.docs.GetByID(doc.ID); err == nil {
			doc = fresh
		}
		respond(http.StatusCreated, doc)
	}
}

// parseCaptureBody reads title, content, url, tags, and idempotency_key from
// either a JSON body or a URL-encoded form (used by the PWA share_target).
func parseCaptureBody(r *http.Request) (title, content, rawURL string, tags []string, idempotencyKey string) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			return
		}
		title = r.FormValue("title")
		content = r.FormValue("text")
		rawURL = r.FormValue("url")
		idempotencyKey = r.FormValue("idempotency_key")
		return
	}

	// Default: JSON
	var body struct {
		Title          string   `json:"title"`
		Content        string   `json:"content"`
		URL            string   `json:"url"`
		Tags           []string `json:"tags"`
		IdempotencyKey string   `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
		title = body.Title
		content = body.Content
		rawURL = body.URL
		tags = body.Tags
		idempotencyKey = body.IdempotencyKey
	}
	return
}

// ogMeta holds extracted Open Graph metadata.
type ogMeta struct {
	title       string
	description string
}

// fetchOpenGraph fetches the given URL and extracts og:title and og:description
// meta tags. It is best-effort: any failure returns empty strings silently.
func fetchOpenGraph(rawURL string) ogMeta {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ogMeta{}
	}
	req.Header.Set("User-Agent", "PKD/2.0 (+https://github.com/edalcin/pkd)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ogMeta{}
	}
	defer resp.Body.Close()

	// Cap body read at 1 MB to avoid giant pages
	limited := io.LimitReader(resp.Body, 1<<20)
	return parseOGFromHTML(limited)
}

// parseOGFromHTML tokenizes the HTML and extracts og:title and og:description.
func parseOGFromHTML(r io.Reader) ogMeta {
	var meta ogMeta
	z := html.NewTokenizer(r)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		// Stop scanning once we're past </head>
		if tt == html.EndTagToken {
			tok := z.Token()
			if tok.Data == "head" {
				break
			}
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		if tok.Data != "meta" {
			continue
		}
		var prop, content string
		for _, attr := range tok.Attr {
			switch attr.Key {
			case "property":
				prop = attr.Val
			case "content":
				content = attr.Val
			}
		}
		switch prop {
		case "og:title":
			if meta.title == "" {
				meta.title = strings.TrimSpace(content)
			}
		case "og:description":
			if meta.description == "" {
				meta.description = strings.TrimSpace(content)
			}
		}
		if meta.title != "" && meta.description != "" {
			break
		}
	}
	return meta
}
