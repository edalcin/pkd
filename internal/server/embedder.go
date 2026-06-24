package server

import (
	"context"
	"log"
	"time"

	"github.com/edalcin/pkd/internal/store"
)

// embedder runs background embedding sweeps. A single goroutine drains a
// coalescing trigger channel; saves call notify(). sweepInterval is
// configurable via PKD_EMBED_SWEEP_MINUTES (default 15 min).
type embedder struct {
	links         *store.LinkStore
	apiKey        string
	sweepInterval time.Duration
	trigger       chan struct{}
}

func newEmbedder(links *store.LinkStore, apiKey string, sweepInterval time.Duration) *embedder {
	return &embedder{links: links, apiKey: apiKey, sweepInterval: sweepInterval, trigger: make(chan struct{}, 1)}
}

// notify requests a sweep without blocking. Coalesces: second call while one
// is pending is a no-op.
func (e *embedder) notify() {
	if e == nil {
		return
	}
	select {
	case e.trigger <- struct{}{}:
	default:
	}
}

// run sweeps on startup, on every notify, and on the periodic ticker.
// Exits when ctx is cancelled (shutdown). No-op when apiKey is empty.
func (e *embedder) run(ctx context.Context) {
	if e.apiKey == "" {
		return
	}
	e.sweep(ctx)
	t := time.NewTicker(e.sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.trigger:
			e.sweep(ctx)
		case <-t.C:
			e.sweep(ctx)
		}
	}
}

func (e *embedder) sweep(ctx context.Context) {
	n, err := e.links.EmbedStaleDocs(ctx, e.apiKey)
	if err != nil {
		log.Printf("embedder: %v", err)
		return
	}
	if n > 0 {
		log.Printf("embedder: embedded %d document(s)", n)
	}
}
