package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/edalcin/pkd/internal/storage"
)

// SweepStaleTempObjects deletes objects under prefix older than maxAge. Used
// at startup to remove backup ZIPs left behind by a previous process that
// crashed before its inline cleanup could run.
//
// Returns the number of objects deleted and any error encountered while
// listing or deleting. Partial progress is preserved if a batch fails midway.
func SweepStaleTempObjects(ctx context.Context, cap storage.S3Capable, prefix string, maxAge time.Duration) (int, error) {
	items, err := cap.ListWithMetadata(ctx, prefix)
	if err != nil {
		return 0, fmt.Errorf("sweep list: %w", err)
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	stale := make([]string, 0, len(items))
	for _, it := range items {
		if it.LastModified.Before(cutoff) {
			stale = append(stale, it.Key)
		}
	}
	if len(stale) == 0 {
		return 0, nil
	}
	if err := cap.DeleteMany(ctx, stale); err != nil {
		return 0, fmt.Errorf("sweep delete: %w", err)
	}
	return len(stale), nil
}
