package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/edalcin/pkd/internal/store"
)

func (s *Server) handleAdminListTrash() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docs, err := s.docs.ListTrash()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if docs == nil {
			docs = nil // JSON will encode as null; frontend handles it
		}
		writeJSON(w, http.StatusOK, docs)
	}
}

func (s *Server) handleAdminDeleteTrashItem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := s.docs.PermanentDelete(id); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminEmptyTrash() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.docs.EmptyTrash(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminBackup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Write the backup to a temp file so we can stream it without holding the
		// VACUUM INTO operation in memory.
		tmp, err := os.CreateTemp("", "pkd-backup-*.sqlite")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmp.Name())
		tmp.Close()

		if err := s.backup.Backup(tmp.Name()); err != nil {
			http.Error(w, "backup failed", http.StatusInternalServerError)
			return
		}

		f, err := os.Open(tmp.Name())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		stat, _ := f.Stat()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="pkd-backup.sqlite"`)
		if stat != nil {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		}
		io.Copy(w, f)
	}
}

func (s *Server) handleAdminRestore() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(512 << 20); err != nil { // 512 MB limit
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.FormValue("confirm") != "REPLACE" {
			http.Error(w, "confirmation required: send confirm=REPLACE", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		tmp, err := os.CreateTemp("", "pkd-restore-*.sqlite")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName)
		if _, err := io.Copy(tmp, file); err != nil {
			tmp.Close()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tmp.Close()

		if err := s.backup.Restore(tmpName); err != nil {
			http.Error(w, "restore failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Reopen the database after restore
		newDB, err := store.Open(s.cfg.DBPath)
		if err != nil {
			http.Error(w, "restore succeeded but could not reopen database — restart the container", http.StatusInternalServerError)
			return
		}
		s.db = newDB
		s.docs = store.NewDocumentStore(newDB)
		s.attachments = store.NewAttachmentStore(newDB, s.cfg.AttachmentsPath)
		s.tags = store.NewTagStore(newDB)
		s.search = store.NewSearchStore(newDB)
		s.shares = store.NewShareStore(newDB)
		s.backup = store.NewBackupStore(newDB, s.cfg.DBPath)

		// Invalidate all sessions (user must log in again)
		s.sessions = s.sessions.Reset()

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminCleanup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orphans, err := s.attachments.ListOrphanedStoredFiles()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		removed := 0
		for _, path := range orphans {
			if os.Remove(path) == nil {
				removed++
			}
		}
		if _, err := s.db.Exec("VACUUM"); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"orphans_removed": removed})
	}
}

func (s *Server) handleAdminRenameTag() http.HandlerFunc {
	type request struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.tags.RenameOrMerge(req.Old, req.New); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "tag not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
