package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/edalcin/pkd/internal/storage"
)

type storageConfigResponse struct {
	Backend      string `json:"backend"`       // "local" or "s3"
	S3Configured bool   `json:"s3_configured"` // true when S3 env vars are present
	S3Bucket     string `json:"s3_bucket,omitempty"`
	S3Region     string `json:"s3_region,omitempty"`
	S3Prefix     string `json:"s3_prefix,omitempty"`
	AuthMethod   string `json:"auth_method,omitempty"` // "instance_profile" or "access_key"
}

func (s *Server) handleAdminStorageConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		active := s.activeStorage()
		resp := storageConfigResponse{
			Backend:      active.Name(),
			S3Configured: s.s3Backend != nil,
		}
		if s.cfg.S3 != nil {
			resp.S3Bucket = s.cfg.S3.Bucket
			resp.S3Region = s.cfg.S3.Region
			resp.S3Prefix = s.cfg.S3.Prefix
			if s.cfg.S3.AccessKeyID != "" {
				resp.AuthMethod = "access_key"
			} else {
				resp.AuthMethod = "instance_profile"
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (s *Server) handleAdminStorageSetBackend() http.HandlerFunc {
	type request struct {
		Backend string `json:"backend"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Backend != "local" && req.Backend != "s3" {
			http.Error(w, `backend must be "local" or "s3"`, http.StatusBadRequest)
			return
		}
		if err := s.setActiveStorage(req.Backend); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		active := s.activeStorage()
		writeJSON(w, http.StatusOK, map[string]string{"backend": active.Name()})
	}
}

type testResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleAdminStorageTest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		results := map[string]*testResult{
			"local": testBackend(ctx, s.localBackend),
		}
		if s.s3Backend != nil {
			results["s3"] = testBackend(ctx, s.s3Backend)
		}
		writeJSON(w, http.StatusOK, results)
	}
}

func testBackend(ctx context.Context, b storage.Backend) *testResult {
	start := time.Now()
	key := fmt.Sprintf(".pkd-healthcheck-%d", time.Now().UnixNano())
	payload := []byte("pkd-healthcheck")

	if err := b.Put(ctx, key, bytes.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		return &testResult{OK: false, LatencyMS: ms(start), Error: "put failed: " + err.Error()}
	}
	rc, _, err := b.Get(ctx, key)
	if err != nil {
		b.Delete(ctx, key)
		return &testResult{OK: false, LatencyMS: ms(start), Error: "get failed: " + err.Error()}
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(got) != string(payload) {
		b.Delete(ctx, key)
		return &testResult{OK: false, LatencyMS: ms(start), Error: "verify failed: content mismatch"}
	}
	b.Delete(ctx, key)
	return &testResult{OK: true, LatencyMS: ms(start)}
}

func ms(start time.Time) int64 { return time.Since(start).Milliseconds() }

// handleAdminStorageBackupAttachments is the legacy synchronous backup endpoint.
// Streams a flat ZIP of every key in the local backend directly to the HTTP
// response. Kept for backwards compatibility with the original UI flow.
// New code should use the async pipeline at
// POST /api/admin/storage/backup-start, which supports both local and S3
// backends, deduplicates by SHA256, emits a manifest, and reports progress.
func (s *Server) handleAdminStorageBackupAttachments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		keys, err := s.localBackend.List(ctx, "")
		if err != nil {
			http.Error(w, "list error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		filename := "pkd-attachments-" + time.Now().Format("2006-01-02") + ".zip"
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		zw := zip.NewWriter(w)
		defer zw.Close()
		for _, key := range keys {
			rc, _, err := s.localBackend.Get(ctx, key)
			if err != nil {
				continue
			}
			fw, err := zw.Create(key)
			if err != nil {
				rc.Close()
				continue
			}
			io.Copy(fw, rc)
			rc.Close()
		}
	}
}

// handleAdminStorageRestoreAttachments is the legacy synchronous restore
// endpoint. Accepts a flat ZIP and writes every entry to the local backend
// using the entry name as the storage key. New code should use the async
// pipeline at POST /api/admin/storage/restore-start, which validates against
// the attachments table (skips orphans), supports cross-backend restore,
// per-entry conflict resolution (overwrite/keep/abort), and integrity
// verification by SHA256.
func (s *Server) handleAdminStorageRestoreAttachments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := r.ParseMultipartForm(200 << 20); err != nil {
			http.Error(w, "form parse error: "+err.Error(), http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			http.Error(w, "invalid zip: "+err.Error(), http.StatusBadRequest)
			return
		}
		restored := 0
		var errs []string
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				errs = append(errs, zf.Name+": "+err.Error())
				continue
			}
			err = s.localBackend.Put(ctx, zf.Name, rc, int64(zf.UncompressedSize64), "application/octet-stream")
			rc.Close()
			if err != nil {
				errs = append(errs, zf.Name+": "+err.Error())
				continue
			}
			restored++
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"restored": restored,
			"errors":   errs,
		})
	}
}

func (s *Server) handleAdminStorageMigrateStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind := s.currentBackendKind()
		job, err := s.jobs.Start("migrate", kind, 0)
		if err != nil {
			if errors.Is(err, ErrJobInFlight) {
				http.Error(w, "Já existe uma operação em andamento para este backend", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go s.runMigrateJob(job)
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
	}
}

func (s *Server) runMigrateJob(job *Job) {
	defer func() {
		if r := recover(); r != nil {
			s.jobs.Finish(job, "failed", fmt.Sprintf("panic: %v", r))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	target := s.activeStorage()
	totalFound, copied, errs := s.attachments.MigrateToBackend(ctx, target, func(processed, total int64) {
		atomic.StoreInt64(&job.Total, total)
		atomic.StoreInt64(&job.Processed, processed)
	})
	job.StorageOp = &StorageOpSummary{
		TotalFound: int64(totalFound),
		Succeeded:  int64(copied),
		Errors:     errs,
	}
	s.jobs.Finish(job, "succeeded", "")
}

func (s *Server) handleAdminStorageReconcileStart() http.HandlerFunc {
	type request struct {
		Backend string `json:"backend"` // "s3" | "local"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Backend == "" {
			req.Backend = "s3" // default
		}

		var src storage.Backend
		switch req.Backend {
		case "s3":
			if s.s3Backend == nil {
				http.Error(w, "S3 not configured", http.StatusBadRequest)
				return
			}
			src = s.s3Backend
		case "local":
			src = s.localBackend
		default:
			http.Error(w, `backend must be "s3" or "local"`, http.StatusBadRequest)
			return
		}

		// Gate key: "reconcile-<backend>" keeps reconcile ops independent of
		// backup/migrate jobs while still preventing duplicate reconcile runs.
		gateKey := "reconcile-" + req.Backend
		job, err := s.jobs.Start("reconcile", gateKey, 0)
		if err != nil {
			if errors.Is(err, ErrJobInFlight) {
				http.Error(w, "Já existe uma operação em andamento para este backend", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go s.runReconcileJob(job, src)
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
	}
}

func (s *Server) runReconcileJob(job *Job, src storage.Backend) {
	defer func() {
		if r := recover(); r != nil {
			s.jobs.Finish(job, "failed", fmt.Sprintf("panic: %v", r))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fixed, errs := s.attachments.ReconcileStorageLocations(ctx, src, func(processed, total int64) {
		atomic.StoreInt64(&job.Total, total)
		atomic.StoreInt64(&job.Processed, processed)
	})
	job.StorageOp = &StorageOpSummary{
		TotalFound: job.Total,
		Succeeded:  int64(fixed),
		Errors:     errs,
	}
	s.jobs.Finish(job, "succeeded", "")
}

func (s *Server) handleAdminStorageCleanupSourceStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := s.activeStorage()

		var source storage.Backend
		switch target.Name() {
		case "s3":
			source = s.localBackend
		case "local":
			if s.s3Backend == nil {
				http.Error(w, "S3 not configured", http.StatusBadRequest)
				return
			}
			source = s.s3Backend
		}

		kind := s.currentBackendKind()
		job, err := s.jobs.Start("cleanup", kind, 0)
		if err != nil {
			if errors.Is(err, ErrJobInFlight) {
				http.Error(w, "Já existe uma operação em andamento para este backend", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go s.runCleanupJob(job, source, target)
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
	}
}

func (s *Server) runCleanupJob(job *Job, source, target storage.Backend) {
	defer func() {
		if r := recover(); r != nil {
			s.jobs.Finish(job, "failed", fmt.Sprintf("panic: %v", r))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	removed, errs := s.attachments.CleanupSource(ctx, source, target, func(processed, total int64) {
		atomic.StoreInt64(&job.Total, total)
		atomic.StoreInt64(&job.Processed, processed)
	})
	job.StorageOp = &StorageOpSummary{
		TotalFound: job.Total,
		Succeeded:  int64(removed),
		Errors:     errs,
	}
	s.jobs.Finish(job, "succeeded", "")
}
