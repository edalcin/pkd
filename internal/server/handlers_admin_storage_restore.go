package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/edalcin/pkd/internal/backup"
	"github.com/edalcin/pkd/internal/storage"
	"github.com/edalcin/pkd/internal/store"
)

// restoreUploadLimit caps the size of an uploaded ZIP in a single request.
// Matches the existing /api/admin/restore (DB restore) ceiling. Operators
// uploading larger backups should split or restore via direct S3 mc cp +
// re-trigger; out of scope for this version.
const restoreUploadLimit = 512 << 20 // 512 MiB

// attachmentLookup adapts AttachmentStore to backup.AttachmentLookup.
type attachmentLookup struct{ s *store.AttachmentStore }

func (l *attachmentLookup) LookupBySHA256(ctx context.Context, sha string) ([]backup.LookupRef, error) {
	refs, err := l.s.LookupBySHA256(ctx, sha)
	if err != nil {
		return nil, err
	}
	out := make([]backup.LookupRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, backup.LookupRef{
			ID:              r.ID,
			StoredFilename:  r.StoredFilename,
			StorageLocation: r.StorageLocation,
			MimeType:        r.MimeType,
		})
	}
	return out, nil
}

func (s *Server) handleAdminStorageRestoreStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart with bounded request body so a malicious or buggy
		// client cannot exhaust memory before the limit kicks in.
		r.Body = http.MaxBytesReader(w, r.Body, restoreUploadLimit+1<<20)
		if err := r.ParseMultipartForm(restoreUploadLimit); err != nil {
			http.Error(w, "form parse error: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("zip")
		if err != nil {
			http.Error(w, "missing zip field", http.StatusBadRequest)
			return
		}
		defer file.Close()
		onConflict := r.FormValue("on_conflict")
		if onConflict == "" {
			onConflict = backup.ConflictOverwrite
		}

		kind := s.currentBackendKind()
		job, err := s.jobs.Start("restore", kind, 0)
		if err != nil {
			if errors.Is(err, ErrJobInFlight) {
				http.Error(w, "Já existe uma operação em andamento para este backend", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// We can't keep the *multipart.File alive past the response — copy
		// upload to a stable source (S3 temp object or local temp file) and
		// release the request.
		var (
			zipSrc        io.ReaderAt
			zipSize       int64
			cleanup       func()
			prepareErr    error
		)
		switch active := s.activeBackend.(type) {
		case storage.S3Capable:
			zipSrc, zipSize, cleanup, prepareErr = prepareRestoreSourceS3(r.Context(), job, active, file, header.Size)
		default:
			zipSrc, zipSize, cleanup, prepareErr = prepareRestoreSourceLocal(file, header.Size, s.cfg.AttachmentsPath)
			_ = active
		}
		if prepareErr != nil {
			s.jobs.Finish(job, "failed", "stage zip: "+prepareErr.Error())
			http.Error(w, "stage zip: "+prepareErr.Error(), http.StatusInternalServerError)
			return
		}

		go s.runRestoreJob(job, zipSrc, zipSize, onConflict, cleanup)
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
	}
}

// prepareRestoreSourceS3 stream-uploads the request body to a temp object in
// S3 and returns an io.ReaderAt backed by Range GETs against that object.
// cleanup deletes the temp object.
func prepareRestoreSourceS3(ctx context.Context, job *Job, cap storage.S3Capable, body io.Reader, expectedSize int64) (io.ReaderAt, int64, func(), error) {
	tempKey := backupTempPrefix + job.ID + "-restore.zip"
	job.TempKey = tempKey
	if err := cap.UploadFromReader(ctx, tempKey, body, "application/zip"); err != nil {
		return nil, 0, nil, fmt.Errorf("upload to s3: %w", err)
	}
	size, err := cap.HeadSize(context.Background(), tempKey)
	if err != nil {
		_ = cap.DeleteMany(context.Background(), []string{tempKey})
		return nil, 0, nil, fmt.Errorf("head: %w", err)
	}
	if expectedSize > 0 && size != expectedSize {
		log.Printf("restore job %s: size mismatch (multipart said %d, S3 reports %d)", job.ID, expectedSize, size)
	}
	cleanup := func() {
		_ = cap.DeleteMany(context.Background(), []string{tempKey})
	}
	return backup.NewS3RangeReaderAt(context.Background(), cap, tempKey), size, cleanup, nil
}

// prepareRestoreSourceLocal writes the upload to a temp file under the local
// attachments dir (which is writable and sized for the workload) and hands
// back an *os.File as io.ReaderAt.
func prepareRestoreSourceLocal(body io.Reader, expectedSize int64, attachmentsDir string) (io.ReaderAt, int64, func(), error) {
	f, err := os.CreateTemp(attachmentsDir, "restore-*.zip")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("temp file: %w", err)
	}
	n, err := io.Copy(f, body)
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, 0, nil, fmt.Errorf("copy upload: %w", err)
	}
	_ = expectedSize
	cleanup := func() {
		f.Close()
		os.Remove(f.Name())
	}
	return f, n, cleanup, nil
}

// runRestoreJob is the worker goroutine spawned by handleAdminStorageRestoreStart.
// Always runs cleanup, even on panic or failure (FR-009).
func (s *Server) runRestoreJob(job *Job, zipSrc io.ReaderAt, zipSize int64, onConflict string, cleanup func()) {
	defer func() {
		if cleanup != nil {
			cleanup()
		}
		if r := recover(); r != nil {
			s.jobs.Finish(job, "failed", fmt.Sprintf("panic: %v", r))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	res, err := backup.StreamingRestore(
		ctx,
		zipSrc,
		zipSize,
		s.activeBackend,
		&attachmentLookup{s: s.attachments},
		backup.RestoreOptions{
			OnConflict: onConflict,
			OnProgress: func(processed int64) {
				atomic.StoreInt64(&job.Processed, processed)
			},
		},
	)
	atomic.StoreInt64(&job.Total, res.ManifestEntries)
	atomic.StoreInt64(&job.SizeBytes, res.BytesWritten)
	job.Restore = &RestoreSummary{
		Written:       res.Written,
		Kept:          res.Kept,
		SkippedOrphan: res.SkippedOrphan,
		HashMismatch:  res.HashMismatch,
		Skipped:       res.Skipped,
	}
	if err != nil {
		s.jobs.Finish(job, "failed", err.Error())
		return
	}
	s.jobs.Finish(job, "succeeded", "")
	log.Printf("restore job %s: %d entries, %d written, %d kept, %d orphan, %d hash-mismatch, %d bytes",
		job.ID, res.ManifestEntries, res.Written, res.Kept, res.SkippedOrphan, res.HashMismatch, res.BytesWritten)
}
