package server

import (
	"errors"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/edalcin/pkd/internal/store"
)

func (s *Server) handleCreateAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

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
			att, err := s.attachments.CreateFile(docID, origName, mimeType, file, maxBytes)
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

		att, err := s.attachments.CreateFile(docID, origName, mimeType, r.Body, maxBytes)
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
			"url":    att.URL,
			"id":     att.ID,
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
		fullPath, err := s.attachments.FullPath(att)
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", att.MimeType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+att.OriginalName+`"`)
		http.ServeFile(w, r, fullPath)
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
