package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (s *Server) handleAdminStorageMigrate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := s.activeStorage()
		start := time.Now()

		copied, errs := s.attachments.MigrateToBackend(r.Context(), target)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"target":      target.Name(),
			"copied":      copied,
			"errors":      errs,
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}
}

func (s *Server) handleAdminStorageCleanupSource() http.HandlerFunc {
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

		removed, errs := s.attachments.CleanupSource(r.Context(), source, target)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"removed": removed,
			"errors":  errs,
		})
	}
}
