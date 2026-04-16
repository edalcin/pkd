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

	"github.com/edalcin/pkd/internal/security"
)

// handleCapture serves POST /api/capture.
// Accepts both application/json and application/x-www-form-urlencoded (PWA share_target).
// Creates a new document from the captured content and tags it with #captura.
func (s *Server) handleCapture() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title, content, rawURL, extraTags := parseCaptureBody(r)

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

		// Create the document
		doc, err := s.docs.Create(nil, title)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Save body
		doc, err = s.docs.Update(doc.ID, doc.Version, title, safeHTML, plainText, "")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Apply tags: always add #captura plus any extras
		allTags := append([]string{"captura"}, extraTags...)
		if err := s.tags.SetDocumentTags(doc.ID, allTags); err != nil {
			// Non-fatal — document is still created
			_ = err
		}

		// Re-fetch with tags included
		doc, err = s.docs.GetByID(doc.ID)
		if err != nil {
			writeJSON(w, http.StatusCreated, doc)
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	}
}

// parseCaptureBody reads title, content, url, and tags from either a JSON body
// or a URL-encoded form (used by the PWA share_target).
func parseCaptureBody(r *http.Request) (title, content, rawURL string, tags []string) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			return
		}
		title = r.FormValue("title")
		content = r.FormValue("text")
		rawURL = r.FormValue("url")
		return
	}

	// Default: JSON
	var body struct {
		Title   string   `json:"title"`
		Content string   `json:"content"`
		URL     string   `json:"url"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
		title = body.Title
		content = body.Content
		rawURL = body.URL
		tags = body.Tags
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
