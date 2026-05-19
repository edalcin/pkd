package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/edalcin/pkd/internal/backup"
	"github.com/edalcin/pkd/internal/storage"
	"github.com/edalcin/pkd/internal/store"
)

// presignedDownloadTTL is how long a backup download URL is valid for.
// Short by design — admin downloads the ZIP immediately or regenerates the URL.
const presignedDownloadTTL = 15 * time.Minute

// attachmentSource resolves attachment content from the appropriate backend.
type attachmentSource struct {
	local storage.Backend
	s3    storage.Backend
}

func (a *attachmentSource) Open(ctx context.Context, att backup.Attachment) (io.ReadCloser, error) {
	var b storage.Backend
	switch att.StorageLocation {
	case "s3":
		b = a.s3
	default:
		b = a.local
	}
	if b == nil {
		return nil, fmt.Errorf("backend %q unavailable", att.StorageLocation)
	}
	rc, _, err := b.Get(ctx, att.StoredFilename)
	return rc, err
}

// shaPersister adapts AttachmentStore.BackfillSHA256 to backup.SHA256Persister.
type shaPersister struct{ s *store.AttachmentStore }

func (p *shaPersister) BackfillSHA256(ctx context.Context, id int64, sha string) error {
	return p.s.BackfillSHA256(ctx, id, sha)
}

func (s *Server) handleAdminStorageBackupStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind := s.currentBackendKind()
		// Only S3 backend gets the new streaming path. Local backend keeps the
		// legacy synchronous endpoint, which the UI continues to surface.
		if kind != "s3" {
			http.Error(w, "async backup only supported for S3 backend", http.StatusNotImplemented)
			return
		}
		cap, ok := s.activeBackend.(storage.S3Capable)
		if !ok {
			http.Error(w, "active backend does not support streaming uploads", http.StatusInternalServerError)
			return
		}

		job, err := s.jobs.Start("backup", kind, 0)
		if err != nil {
			if errors.Is(err, ErrJobInFlight) {
				http.Error(w, "Já existe uma operação em andamento para este backend", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		go s.runBackupJob(job, cap)
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
	}
}

// runBackupJob is the worker goroutine spawned by handleAdminStorageBackupStart.
// On any error, the temp object is best-effort deleted and the job ends
// "failed". On success, a presigned download URL is attached to the job.
// recover() guards against unhandled panics so the goroutine cannot crash
// the entire process and so the job state always settles.
func (s *Server) runBackupJob(job *Job, cap storage.S3Capable) {
	tempKey := backupTempPrefix + job.ID + ".zip"
	job.TempKey = tempKey

	defer func() {
		if r := recover(); r != nil {
			// Best-effort cleanup of the temp object; ignore error because
			// the next-startup sweep will catch it if this fails.
			_ = cap.DeleteMany(context.Background(), []string{tempKey})
			s.jobs.Finish(job, "failed", fmt.Sprintf("panic: %v", r))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	atts, err := s.attachments.EnumerateForBackup(ctx)
	if err != nil {
		s.jobs.Finish(job, "failed", "enumerate attachments: "+err.Error())
		return
	}
	atomic.StoreInt64(&job.Total, int64(len(atts)))

	// Map store rows to the backup package's flat type.
	flat := make([]backup.Attachment, 0, len(atts))
	for _, a := range atts {
		flat = append(flat, backup.Attachment{
			ID:              a.ID,
			StoredFilename:  a.StoredFilename,
			MimeType:        a.MimeType,
			SizeBytes:       a.SizeBytes,
			StorageLocation: a.StorageLocation,
			ContentSHA256:   a.ContentSHA256,
		})
	}

	src := &attachmentSource{local: s.localBackend, s3: s.s3Backend}
	env := backup.EnvDescriptor{BackendKind: "s3"}
	if s.cfg.S3 != nil {
		env.Bucket = s.cfg.S3.Bucket
		env.Prefix = s.cfg.S3.Prefix
	}

	// Pipe writer feeds zip; reader is the multipart upload body.
	pr, pw := io.Pipe()
	uploadErr := make(chan error, 1)
	go func() {
		uploadErr <- cap.UploadFromReader(ctx, tempKey, pr, "application/zip")
	}()

	res, writeErr := backup.StreamingBackup(ctx, pw, backup.WriteOptions{
		Env:         env,
		Source:      src,
		Persister:   &shaPersister{s: s.attachments},
		Attachments: flat,
		OnProgress: func(processed int64) {
			atomic.StoreInt64(&job.Processed, processed)
		},
	})
	// Close pipe writer so uploader's read returns EOF (success) or
	// CloseWithError so it returns the underlying error.
	if writeErr != nil {
		pw.CloseWithError(writeErr)
	} else {
		pw.Close()
	}
	uErr := <-uploadErr

	if writeErr != nil {
		_ = cap.DeleteMany(ctx, []string{tempKey})
		s.jobs.Finish(job, "failed", "compose zip: "+writeErr.Error())
		return
	}
	if uErr != nil {
		_ = cap.DeleteMany(ctx, []string{tempKey})
		s.jobs.Finish(job, "failed", "upload zip: "+uErr.Error())
		return
	}

	atomic.StoreInt64(&job.SizeBytes, res.BytesWritten)

	url, err := cap.PresignGet(ctx, tempKey, presignedDownloadTTL)
	if err != nil {
		_ = cap.DeleteMany(ctx, []string{tempKey})
		s.jobs.Finish(job, "failed", "presign: "+err.Error())
		return
	}
	s.jobs.SetDownloadURL(job.ID, url, time.Now().UTC().Add(presignedDownloadTTL))
	s.jobs.Finish(job, "succeeded", "")
	log.Printf("backup job %s: %d attachments, %d unique entries, %d bytes",
		job.ID, res.TotalAttachments, res.UniqueEntries, res.BytesWritten)
}

func (s *Server) handleAdminStorageGetJob() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, ok := s.jobs.Get(id)
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func (s *Server) handleAdminStorageRegenerateDownloadURL() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, ok := s.jobs.Get(id)
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		if job.Kind != "backup" || job.BackendKind != "s3" {
			http.Error(w, "job is not an S3 backup", http.StatusConflict)
			return
		}
		if job.State != "succeeded" {
			http.Error(w, "job not succeeded", http.StatusNotFound)
			return
		}
		cap, ok := s.activeBackend.(storage.S3Capable)
		if !ok {
			http.Error(w, "active backend changed; temp object not addressable", http.StatusConflict)
			return
		}
		url, err := cap.PresignGet(r.Context(), job.TempKey, presignedDownloadTTL)
		if err != nil {
			http.Error(w, "presign failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		expires := time.Now().UTC().Add(presignedDownloadTTL)
		s.jobs.SetDownloadURL(job.ID, url, expires)
		writeJSON(w, http.StatusOK, map[string]any{
			"download_url":   url,
			"url_expires_at": expires,
		})
	}
}
