package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"strings"

	"github.com/edalcin/pkd/internal/store"
)

// importAttachment is one item of the optional "attachments" array in the
// agent create bodies (/api/documents, /api/notes, /api/memories): a
// base64-encoded file to embed inline in the created item. See ADR-003.
type importAttachment struct {
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	DataBase64 string `json:"data_base64"`
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

// rollbackImportedDocument reverts an item created earlier in the same agent
// request after one of its attachments failed to import
// (ADR-003, decision D3). PermanentDelete only removes trashed documents, so
// the document is soft-deleted first; both steps are best-effort since the
// caller is already reporting the triggering error.
func (s *Server) rollbackImportedDocument(docID int64) {
	_ = s.attachments.DeleteByDocument(docID)
	_ = s.docs.SoftDelete(docID)
	_ = s.docs.PermanentDelete(docID)
}
