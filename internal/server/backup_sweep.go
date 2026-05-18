package server

import (
	"context"
	"log"
	"time"

	"github.com/edalcin/pkd/internal/backup"
	"github.com/edalcin/pkd/internal/storage"
)

// backupTempPrefix is the S3 key prefix reserved for transient backup ZIPs.
// Objects here are owned by the backup/restore subsystem and may be deleted
// at any time by the startup sweep or by inline cleanup in handlers.
const backupTempPrefix = "_backup-tmp/"

// staleTempMaxAge is how old a backup temp object must be before the startup
// sweep removes it. 24h is generous — legitimate backups complete in minutes.
const staleTempMaxAge = 24 * time.Hour

// startBackupTempSweep launches a non-blocking goroutine that removes stale
// backup temp objects from the active S3 backend. Called once during Server
// construction. No-op when the active backend is not S3-capable.
func (s *Server) startBackupTempSweep() {
	cap, ok := s.activeBackend.(storage.S3Capable)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		n, err := backup.SweepStaleTempObjects(ctx, cap, backupTempPrefix, staleTempMaxAge)
		if err != nil {
			log.Printf("backup sweep: %v", err)
			return
		}
		if n > 0 {
			log.Printf("backup sweep: removed %d stale temp object(s)", n)
		}
	}()
}
