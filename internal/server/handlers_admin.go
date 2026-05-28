package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/edalcin/pkd/internal/model"
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
			docs = []*model.Document{}
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
		// Delete attachment files first (FK is RESTRICT, not CASCADE)
		if err := s.attachments.DeleteByDocument(id); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := s.docs.PermanentDelete(id); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Clean up tags that were exclusive to this document
		_ = s.tags.PruneUnused()
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminEmptyTrash() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trashed, err := s.docs.ListTrash()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, doc := range trashed {
			_ = s.attachments.DeleteByDocument(doc.ID)
		}
		if err := s.docs.EmptyTrash(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_ = s.tags.PruneUnused()
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
		s.attachments = store.NewAttachmentStore(newDB, s.localBackend, s.s3Backend)
		s.tags = store.NewTagStore(newDB)
		s.search = store.NewSearchStore(newDB)
		s.shares = store.NewShareStore(newDB)
		s.backup = store.NewBackupStore(newDB, s.cfg.DBPath)
		s.links = store.NewLinkStore(newDB)
		s.urls = store.NewURLStore(newDB)

		// Invalidate all sessions (user must log in again after restore)
		s.sessions = s.sessions.Reset(newDB)

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminCleanup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ListOrphanedStoredFiles returns logical keys (not absolute paths) for local-backend files.
		orphans, err := s.attachments.ListOrphanedStoredFiles()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		removed := 0
		for _, key := range orphans {
			if s.localBackend.Delete(r.Context(), key) == nil {
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

func (s *Server) handleAdminListURLs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.urls.ListAllWithDocTitle()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

func (s *Server) handleAdminCheckURLs() http.HandlerFunc {
	type result struct {
		ID         int64  `json:"id"`
		DocumentID int64  `json:"document_id"`
		URL        string `json:"url"`
		Title      string `json:"title"`
		Valid       bool   `json:"valid"`
		StatusCode  int    `json:"status_code"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		urls, err := s.urls.ListAll()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		client := &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}

		results := make([]result, 0, len(urls))
		for _, u := range urls {
			res := result{
				ID:         u.ID,
				DocumentID: u.DocumentID,
				URL:        u.URL,
				Title:      u.Title,
			}
			resp, err := client.Head(u.URL)
			if err != nil {
				// Try GET as fallback (some servers reject HEAD)
				resp, err = client.Get(u.URL)
				if err != nil {
					res.Valid = false
					res.StatusCode = 0
					results = append(results, res)
					continue
				}
				resp.Body.Close()
			} else {
				resp.Body.Close()
			}
			res.StatusCode = resp.StatusCode
			res.Valid = resp.StatusCode >= 200 && resp.StatusCode < 400
			results = append(results, res)
		}
		writeJSON(w, http.StatusOK, results)
	}
}

func (s *Server) handleAdminListAllTags() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tags, err := s.tags.ListAllWithCounts()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if tags == nil {
			tags = []*model.TagWithCount{}
		}
		writeJSON(w, http.StatusOK, tags)
	}
}

func (s *Server) handleAdminListAttachments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atts, err := s.attachments.ListAllWithDocument()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if atts == nil {
			atts = []*model.AttachmentWithDoc{}
		}
		writeJSON(w, http.StatusOK, atts)
	}
}

func (s *Server) handleAdminListOrphans() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orphans, err := s.attachments.ListOrphanedStoredFiles()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": len(orphans)})
	}
}

func (s *Server) handleAdminUpdateTag() http.HandlerFunc {
	type request struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.tags.Update(id, req.Name, req.Color); err != nil {
			http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminDeleteTag() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := s.tags.Delete(id); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminPruneTags() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		removed, err := s.tags.PruneZeroCount()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"removed": int(removed)})
	}
}

func (s *Server) handleAdminListShares() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shares, err := s.shares.ListAllActive()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if shares == nil {
			shares = []*model.ShareWithDoc{}
		}
		// Build the base URL once and attach the full public URL to each share.
		base := s.cfg.BaseURL
		if base == "" {
			scheme := "http"
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			base = scheme + "://" + r.Host + "/"
		}
		for _, sh := range shares {
			if sh.Token != "" {
				sh.URL = base + "public/" + sh.Token
			}
		}
		writeJSON(w, http.StatusOK, shares)
	}
}

func (s *Server) handleAdminRevokeShare() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shareID, err := parseID(r, "shareID")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
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

// handleAdminGetSettings returns the editable subset of server-side settings.
func (s *Server) handleAdminGetSettings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maxPerDoc, err := s.settings.VersionsMaxPerDoc()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"versions.max_per_doc": maxPerDoc,
		})
	}
}

// handleAdminSetSettings updates a whitelisted server-side setting.
func (s *Server) handleAdminSetSettings() http.HandlerFunc {
	type request struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch req.Key {
		case "versions.max_per_doc":
			n, err := strconv.Atoi(req.Value)
			if err != nil || n < 1 || n > 10000 {
				http.Error(w, "versions.max_per_doc must be an integer between 1 and 10000", http.StatusBadRequest)
				return
			}
			if err := s.settings.SetVersionsMaxPerDoc(req.Value); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "unknown setting key", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
