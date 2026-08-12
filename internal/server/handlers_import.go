package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// importAttachment is one item of the optional "attachments" array in the
// /api/import request body: a base64-encoded file to embed inline in the
// created document. See docs/adr/003-import-de-anexos-do-notas.md.
type importAttachment struct {
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	DataBase64 string `json:"data_base64"`
}

// handleImport serves POST /api/import.
// Authenticated via Bearer token (PKD_IMPORT_TOKEN env var).
// Creates a document from the provided content and applies the #notas tag
// plus any extra tags supplied by the caller. Optional base64-encoded
// attachments are stored and embedded inline in the document body; if any
// attachment fails to import, the whole document is rolled back (ADR-003).
func (s *Server) handleImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title       string             `json:"title"`
			Content     string             `json:"content"`
			Tags        []string           `json:"tags"`
			Attachments []importAttachment `json:"attachments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		title := strings.TrimSpace(body.Title)
		if title == "" {
			title = "Nota " + time.Now().Format("2006-01-02 15:04")
		}

		doc, err := s.docs.Create(nil, title)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		finalContent := body.Content
		if len(body.Attachments) > 0 {
			block, status, importErr := s.importAttachments(r, doc.ID, body.Attachments)
			if importErr != nil {
				s.rollbackImportedDocument(doc.ID)
				http.Error(w, importErr.Error(), status)
				return
			}
			finalContent += block
		}

		safeHTML := security.SanitizeEditorHTML(finalContent)
		plainText := security.ExtractPlainText(safeHTML)

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

// importAttachments decodes and stores each imported attachment, returning
// an HTML block ("<hr><p><strong>Anexos</strong></p>..." + one <p> per item)
// to append to the document body before sanitization. On the first failure
// it returns the HTTP status and error to report; the caller is responsible
// for rolling back the document.
func (s *Server) importAttachments(r *http.Request, docID int64, atts []importAttachment) (string, int, error) {
	backend := s.activeStorage()
	var block strings.Builder
	block.WriteString("<hr><p><strong>Anexos</strong></p>")
	for _, a := range atts {
		data, err := base64.StdEncoding.DecodeString(a.DataBase64)
		if err != nil {
			return "", http.StatusBadRequest, errors.New("invalid attachment data_base64")
		}

		isImage := strings.HasPrefix(a.MimeType, "image/")
		subdir := ""
		maxBytes := s.cfg.MaxAttachmentMB * 1024 * 1024
		if isImage {
			subdir = "inline"
			maxBytes = s.cfg.MaxImageMB * 1024 * 1024
		}

		att, err := s.attachments.CreateFile(r.Context(), backend, docID, a.Filename, a.MimeType, subdir, bytes.NewReader(data), maxBytes)
		if errors.Is(err, store.ErrTooLarge) {
			return "", http.StatusRequestEntityTooLarge, errors.New("attachment too large")
		}
		if err != nil {
			return "", http.StatusInternalServerError, errors.New("internal error")
		}

		escapedName := html.EscapeString(a.Filename)
		if isImage {
			block.WriteString(`<p><img src="` + att.URL + `" alt="` + escapedName + `"></p>`)
		} else {
			block.WriteString(`<p><a href="` + att.URL + `">` + escapedName + `</a></p>`)
		}
	}
	return block.String(), 0, nil
}

// rollbackImportedDocument reverts a document created earlier in the same
// /api/import request after one of its attachments failed to import
// (ADR-003, decision D3). PermanentDelete only removes trashed documents, so
// the document is soft-deleted first; both steps are best-effort since the
// caller is already reporting the triggering error.
func (s *Server) rollbackImportedDocument(docID int64) {
	_ = s.attachments.DeleteByDocument(docID)
	_ = s.docs.SoftDelete(docID)
	_ = s.docs.PermanentDelete(docID)
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
