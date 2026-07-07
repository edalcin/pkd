package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

type OrphanInfo struct {
	Key          string `json:"key"`
	SizeBytes    int64  `json:"size_bytes"`
	AttachmentID int64  `json:"attachment_id,omitempty"` // >0 when DB record exists
	OriginalName string `json:"original_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	Reason       string `json:"reason"`              // "no_db_record" | "trashed_doc" | "no_doc"
	DocID        int64  `json:"doc_id,omitempty"`    // trashed doc id (reason=trashed_doc only)
	DocTitle     string `json:"doc_title,omitempty"` // trashed doc title (reason=trashed_doc only)
}

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
		s.links = store.NewLinkStore(newDB, s.cfg.EmbedModel)
		s.urls = store.NewURLStore(newDB)

		// Invalidate all sessions (user must log in again after restore)
		s.sessions = s.sessions.Reset(newDB)

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAdminCleanup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.db.Exec("VACUUM"); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// DiskUsageResponse holds byte counts for each storage component.
type DiskUsageResponse struct {
	DBBytes          int64 `json:"db_bytes"`
	DBWALBytes       int64 `json:"db_wal_bytes"`
	DBSHMBytes       int64 `json:"db_shm_bytes"`
	AttachmentsBytes int64 `json:"attachments_bytes"`
	TotalBytes       int64 `json:"total_bytes"`
}

func statSize(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.Size()
	}
	return 0
}

// handleAdminDiskUsage reports disk space used by the SQLite database files
// and the local attachments directory.
// GET /api/admin/disk-usage
func (s *Server) handleAdminDiskUsage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := DiskUsageResponse{
			DBBytes:    statSize(s.cfg.DBPath),
			DBWALBytes: statSize(s.cfg.DBPath + "-wal"),
			DBSHMBytes: statSize(s.cfg.DBPath + "-shm"),
		}

		_ = filepath.Walk(s.cfg.AttachmentsPath, func(_ string, fi os.FileInfo, err error) error {
			if err == nil && !fi.IsDir() {
				resp.AttachmentsBytes += fi.Size()
			}
			return nil
		})

		resp.TotalBytes = resp.DBBytes + resp.DBWALBytes + resp.DBSHMBytes + resp.AttachmentsBytes
		writeJSON(w, http.StatusOK, resp)
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
		Valid      bool   `json:"valid"`
		StatusCode int    `json:"status_code"`
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}
		atts, total, err := s.attachments.ListAllWithDocumentPaged(limit, offset)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if atts == nil {
			atts = []*model.AttachmentWithDoc{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": atts, "total": total})
	}
}

func (s *Server) handleAdminListOrphans() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := make([]OrphanInfo, 0)

		// Type 1: local disk files with no attachment record in DB.
		keys, err := s.attachments.ListOrphanedStoredFiles()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, key := range keys {
			var size int64
			rc, sz, err := s.localBackend.Get(r.Context(), key)
			if err == nil {
				rc.Close()
				size = sz
			}
			mt := mime.TypeByExtension(filepath.Ext(key))
			if mt == "" {
				mt = "application/octet-stream"
			}
			result = append(result, OrphanInfo{Key: key, SizeBytes: size, MimeType: mt, Reason: "no_db_record"})
		}

		// Type 2: attachment records whose document is trashed or missing.
		dangling, err := s.attachments.ListDanglingAttachments()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, da := range dangling {
			reason := "no_doc"
			if da.DocTrashed {
				reason = "trashed_doc"
			}
			result = append(result, OrphanInfo{
				Key:          da.StoredFilename,
				SizeBytes:    da.SizeBytes,
				AttachmentID: da.ID,
				OriginalName: da.OriginalName,
				MimeType:     da.MimeType,
				Reason:       reason,
				DocID:        da.DocID,
				DocTitle:     da.DocTitle,
			})
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) handleAdminDownloadOrphan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		if !s.isOrphanKey(r, key) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		rc, size, err := s.localBackend.Get(r.Context(), key)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, path.Base(key)))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		io.Copy(w, rc) //nolint:errcheck
	}
}

func (s *Server) handleAdminDeleteOrphan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}

		// Type 2: dangling attachment record — delete record + file (+ trashed doc if applicable).
		if dangling, err := s.attachments.ListDanglingAttachments(); err == nil {
			for _, da := range dangling {
				if da.StoredFilename != key {
					continue
				}
				if da.DocTrashed && da.DocID > 0 {
					// Permanently delete the trashed document (its attachments have RESTRICT FK).
					if err := s.attachments.DeleteByDocument(da.DocID); err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					if err := s.docs.PermanentDelete(da.DocID); err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					_ = s.tags.PruneUnused()
				} else {
					// No document — just remove the dangling attachment record + file.
					if err := s.attachments.Delete(da.ID); err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Type 1: disk file with no DB record.
		if keys, err := s.attachments.ListOrphanedStoredFiles(); err == nil {
			for _, k := range keys {
				if k == key {
					if err := s.localBackend.Delete(r.Context(), key); err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}

		http.Error(w, "not found", http.StatusNotFound)
	}
}

// isOrphanKey verifies that key is a true orphan (type 1: no DB record) or
// a dangling attachment (type 2: document trashed or missing).
func (s *Server) isOrphanKey(r *http.Request, key string) bool {
	if keys, err := s.attachments.ListOrphanedStoredFiles(); err == nil {
		for _, k := range keys {
			if k == key {
				return true
			}
		}
	}
	if dangling, err := s.attachments.ListDanglingAttachments(); err == nil {
		for _, da := range dangling {
			if da.StoredFilename == key {
				return true
			}
		}
	}
	return false
}

// handleAdminDeleteAllOrphans deletes every orphan (type 1 + type 2) in one call.
// DELETE /api/admin/attachments/orphans
func (s *Server) handleAdminDeleteAllOrphans() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deleted, failed := 0, 0

		// Type 2: dangling attachment records (document trashed or missing).
		dangling, err := s.attachments.ListDanglingAttachments()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, da := range dangling {
			if da.DocTrashed && da.DocID > 0 {
				if err := s.attachments.DeleteByDocument(da.DocID); err != nil {
					failed++
					continue
				}
				if err := s.docs.PermanentDelete(da.DocID); err != nil {
					failed++
					continue
				}
				_ = s.tags.PruneUnused()
			} else {
				if err := s.attachments.Delete(da.ID); err != nil {
					failed++
					continue
				}
			}
			deleted++
		}

		// Type 1: disk files with no DB record.
		keys, err := s.attachments.ListOrphanedStoredFiles()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, key := range keys {
			if err := s.localBackend.Delete(r.Context(), key); err != nil {
				failed++
				continue
			}
			deleted++
		}

		writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted, "failed": failed})
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

// handleAdminGetSettings returns the editable subset of server-side settings
// plus read-only embedding configuration from the current process config.
func (s *Server) handleAdminGetSettings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maxPerDoc, err := s.settings.VersionsMaxPerDoc()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var embedCount int
		_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM document_embeddings`).Scan(&embedCount)
		keyConfigured := "false"
		if s.cfg.GeminiAPIKey != "" {
			keyConfigured = "true"
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"versions.max_per_doc": maxPerDoc,
			"embed.model":          s.cfg.EmbedModel,
			"embed.sweep_minutes":  strconv.Itoa(s.cfg.EmbedSweepMinutes),
			"embed.key_configured": keyConfigured,
			"embed.count":          strconv.Itoa(embedCount),
			"email_2fa_enabled":    strconv.FormatBool(s.emailEnabled),
		})
	}
}

// handleAdminForgetDevices revokes every trusted 2FA device, forcing a code
// prompt on the next login from any browser.
func (s *Server) handleAdminForgetDevices() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.devices.ForgetAll(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
		case "embed.model":
			if !isValidEmbedModel(req.Value) {
				http.Error(w, "invalid embed model", http.StatusBadRequest)
				return
			}
			if err := s.settings.SetEmbedModel(req.Value); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			s.cfg.EmbedModel = req.Value
			s.links.SetEmbedModel(req.Value)
			s.embedder.notify() // trigger re-embed with new model
		default:
			http.Error(w, "unknown setting key", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// validEmbedModels is the whitelist of Gemini models supported for embeddings.
// ponytail: hardcoded — model list changes rarely; no DB/API lookup needed.
var validEmbedModels = []string{
	"models/gemini-embedding-001",
	"models/text-embedding-004",
	"models/embedding-001",
}

func isValidEmbedModel(model string) bool {
	for _, m := range validEmbedModels {
		if m == model {
			return true
		}
	}
	return false
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

// ExternalImagesDoc describes a document that contains external image URLs.
type ExternalImagesDoc struct {
	DocID    int64  `json:"doc_id"`
	DocTitle string `json:"doc_title"`
	Count    int    `json:"count"`
}

// handleAdminListExternalImages scans all non-trashed documents and returns
// those containing <img src="http..."> pointing outside the app.
// GET /api/admin/external-images
func (s *Server) handleAdminListExternalImages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docs, err := s.docs.ListWithBody()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		result := make([]ExternalImagesDoc, 0)
		for _, d := range docs {
			srcs := extractExternalImageSrcs(d.BodyHTML)
			if len(srcs) > 0 {
				result = append(result, ExternalImagesDoc{
					DocID:    d.ID,
					DocTitle: d.Title,
					Count:    len(srcs),
				})
			}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// ImportResult is returned by the import-external-images endpoint.
type ImportResult struct {
	Imported int `json:"imported"`
	Failed   int `json:"failed"`
}

// handleAdminImportExternalImages downloads all external images in one
// document, saves them as attachments, and rewrites the document body.
// POST /api/admin/documents/{id}/import-external-images
func (s *Server) handleAdminImportExternalImages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.GetByID(docID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		newHTML, imported, failed := s.importExternalImages(r.Context(), docID, doc.BodyHTML)
		if imported > 0 {
			plainText := security.ExtractPlainText(newHTML)
			doc, err = s.docs.Update(docID, doc.Version, doc.Title, newHTML, plainText, doc.Icon)
			if err != nil {
				http.Error(w, "failed to save document", http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, ImportResult{Imported: imported, Failed: failed})
	}
}

type AdminStats struct {
	DocCount         int64                 `json:"doc_count"`
	DocCountActive   int64                 `json:"doc_count_active"`
	DocCountArchived int64                 `json:"doc_count_archived"`
	FileCount        int64                 `json:"file_count"`
	LinkCount        int64                 `json:"link_count"`
	TagCount         int64                 `json:"tag_count"`
	TagStats         []*model.TagDocStats  `json:"tag_stats"`
	RootStats        []*model.RootDocStats `json:"root_stats"`
}

// handleAdminStats returns KB counts: documents (active/archived split),
// attachments, links, tags, plus per-tag and per-root-document breakdowns.
// GET /api/admin/stats
func (s *Server) handleAdminStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var st AdminStats
		qs := []struct {
			dest  *int64
			query string
		}{
			{&st.DocCountActive, `SELECT COUNT(*) FROM documents WHERE trashed_at IS NULL AND archived_at IS NULL`},
			{&st.DocCountArchived, `SELECT COUNT(*) FROM documents WHERE trashed_at IS NULL AND archived_at IS NOT NULL`},
			{&st.FileCount, `SELECT COUNT(*) FROM attachments`},
			{&st.LinkCount, `SELECT COUNT(*) FROM document_links`},
			{&st.TagCount, `SELECT COUNT(*) FROM tags`},
		}
		for _, q := range qs {
			if err := s.db.QueryRowContext(r.Context(), q.query).Scan(q.dest); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		st.DocCount = st.DocCountActive + st.DocCountArchived

		var err error
		if st.TagStats, err = s.tags.StatsByTag(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if st.RootStats, err = s.docs.RootStats(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}
}
