package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/edalcin/pkd/internal/storage"
	"github.com/edalcin/pkd/internal/store"
)

// isInlineableMIME returns true for MIME types browsers can display natively.
func isInlineableMIME(mimeType string) bool {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return true
	case strings.HasPrefix(mimeType, "text/"):
		return true
	case strings.HasPrefix(mimeType, "audio/"):
		return true
	case strings.HasPrefix(mimeType, "video/"):
		return true
	case mimeType == "application/pdf":
		return true
	default:
		return false
	}
}

func (s *Server) handleListAttachments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		atts, err := s.attachments.ListByDocument(docID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, atts)
	}
}

func (s *Server) handleCreateAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		backend := s.activeStorage()

		// Accept both multipart/form-data (from the attachment panel) and
		// application/octet-stream (from the CKEditor image upload adapter).
		var (
			origName string
			mimeType string
			maxBytes int64
		)

		ct := r.Header.Get("Content-Type")
		if ct == "application/octet-stream" || ct == "" {
			// CKEditor SimpleUploadAdapter path
			origName = r.Header.Get("X-File-Name")
			if origName == "" {
				origName = "image.png"
			}
			mimeType = mime.TypeByExtension(filepath.Ext(origName))
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			maxBytes = s.cfg.MaxImageMB * 1024 * 1024
		} else {
			// multipart/form-data
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "missing file field", http.StatusBadRequest)
				return
			}
			defer file.Close()
			origName = header.Filename
			mimeType = header.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = mime.TypeByExtension(filepath.Ext(origName))
			}
			maxBytes = s.cfg.MaxAttachmentMB * 1024 * 1024
			subdir := ""
			if r.URL.Query().Get("inline") == "1" {
				subdir = "inline"
			}
			att, err := s.attachments.CreateFile(r.Context(), backend, docID, origName, mimeType, subdir, file, maxBytes)
			if errors.Is(err, store.ErrTooLarge) {
				http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
				return
			}
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, att)
			return
		}

		att, err := s.attachments.CreateFile(r.Context(), backend, docID, origName, mimeType, "", r.Body, maxBytes)
		if errors.Is(err, store.ErrTooLarge) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// CKEditor SimpleUploadAdapter expects: {"default": "<url>"}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"url":     att.URL,
			"id":      att.ID,
			"default": att.URL,
		})
	}
}

func (s *Server) handleGetAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		w.Header().Set("Content-Type", att.MimeType)
		inline := isInlineableMIME(att.MimeType)
		disposition := "attachment"
		if inline {
			disposition = "inline"
		}
		safeName := strings.NewReplacer(`"`, `'`, `\`, `-`).Replace(att.OriginalName)
		w.Header().Set("Content-Disposition", disposition+`; filename="`+safeName+`"`)

		// Override global security headers for inline-displayable attachments.
		if inline {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		}

		// Route to the correct backend based on storage_location.
		// Local backend: supports HTTP range requests via ServeContent.
		// S3 backend: streams without range support (v1).
		backend := s.attachments.BackendForLocation(att.StorageLocation)

		if seeker, ok := backend.(storage.Seeker); ok {
			// Local: full range request support
			f, err := seeker.OpenSeek(r.Context(), att.StoredFilename)
			if err != nil {
				http.Error(w, "file not found", http.StatusNotFound)
				return
			}
			defer f.Close()
			http.ServeContent(w, r, att.OriginalName, att.CreatedAt, f)
		} else {
			// S3: stream without range
			rc, size, err := backend.Get(r.Context(), att.StoredFilename)
			if err != nil {
				http.Error(w, "file not found", http.StatusNotFound)
				return
			}
			defer rc.Close()
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
			w.WriteHeader(http.StatusOK)
			io.Copy(w, rc) //nolint:errcheck
		}
	}
}

func (s *Server) handleDeleteAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := s.attachments.Delete(id); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
