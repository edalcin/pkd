package server

import (
	"context"
	"database/sql"
	"io/fs"
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/edalcin/pkd/internal/config"
	"github.com/edalcin/pkd/internal/sessions"
	"github.com/edalcin/pkd/internal/storage"
	"github.com/edalcin/pkd/internal/store"
)

// Server wraps the HTTP router and all its dependencies.
type Server struct {
	cfg         *config.Config
	db          *sql.DB
	sessions    *sessions.Store
	docs        *store.DocumentStore
	attachments *store.AttachmentStore
	tags        *store.TagStore
	search      *store.SearchStore
	shares      *store.ShareStore
	backup      *store.BackupStore
	links       *store.LinkStore
	urls        *store.URLStore
	settings    *store.SettingsStore
	throttle    *Throttle
	jobs        *BackupJobManager
	handler     http.Handler

	localBackend storage.Backend // always non-nil
	s3Backend    storage.Backend // nil when S3 not configured
	activeMu     sync.RWMutex
	activeBackend storage.Backend // points to localBackend or s3Backend
}

// currentBackendKind returns the active backend identifier ("local" or "s3").
// Used by job scheduling to enforce single-in-flight per backend.
func (s *Server) currentBackendKind() string {
	return s.activeStorage().Name()
}

// activeStorage returns the currently active storage backend (thread-safe).
func (s *Server) activeStorage() storage.Backend {
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()
	return s.activeBackend
}

// setActiveStorage atomically swaps the active backend and persists the choice.
func (s *Server) setActiveStorage(name string) error {
	var b storage.Backend
	switch name {
	case "s3":
		if s.s3Backend == nil {
			return &errS3NotConfigured{}
		}
		b = s.s3Backend
	default:
		name = "local"
		b = s.localBackend
	}
	if err := s.settings.SetAttachmentsBackend(name); err != nil {
		return err
	}
	s.activeMu.Lock()
	s.activeBackend = b
	s.activeMu.Unlock()
	return nil
}

type errS3NotConfigured struct{}

func (e *errS3NotConfigured) Error() string { return "S3 not configured (missing PKD_S3_BUCKET or PKD_S3_REGION)" }

// New builds the chi router with all middleware and routes wired up.
func New(cfg *config.Config, db *sql.DB, sess *sessions.Store) *Server {
	local := storage.NewLocal(cfg.AttachmentsPath)

	var s3b storage.Backend
	if cfg.S3 != nil {
		s3b = buildS3Backend(cfg.S3)
	}

	s := &Server{
		cfg:          cfg,
		db:           db,
		sessions:     sess,
		docs:         store.NewDocumentStore(db),
		tags:         store.NewTagStore(db),
		search:       store.NewSearchStore(db),
		shares:       store.NewShareStore(db),
		backup:       store.NewBackupStore(db, cfg.DBPath),
		links:        store.NewLinkStore(db),
		urls:         store.NewURLStore(db),
		settings:     store.NewSettingsStore(db),
		throttle:     NewThrottle(cfg.TrustProxyHeaders),
		jobs:         NewBackupJobManager(),
		localBackend: local,
		s3Backend:    s3b,
		activeBackend: local, // default; overridden below from DB
	}
	s.attachments = store.NewAttachmentStore(db, local, s3b)

	// Restore active backend from DB settings.
	if name, err := s.settings.AttachmentsBackend(); err == nil && name == "s3" && s3b != nil {
		s.activeBackend = s3b
	}

	// Best-effort startup sweep of orphaned backup temp objects (only S3-capable
	// backends). Non-blocking — does not delay HTTP listener startup.
	s.startBackupTempSweep()

	s.handler = s.buildRouter()
	return s
}

// buildS3Backend creates the S3 client and backend from config.
func buildS3Backend(cfg *config.S3Config) storage.Backend {
	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(cfg.Region))
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		log.Printf("warn: S3 config load failed: %v — S3 backend unavailable", err)
		return nil
	}
	client := awss3.NewFromConfig(awsCfg)
	return storage.NewS3(client, cfg.Bucket, cfg.Prefix)
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ExportThrottle returns the throttle for use in integration tests.
func (s *Server) ExportThrottle() *Throttle { return s.throttle }

func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(SecurityHeaders)
	r.Use(CSRF)

	root := webRoot()

	// Public unauthenticated routes
	r.Get("/healthz", (&healthHandler{db: s.db}).ServeHTTP)
	r.Post("/api/login", s.handleLogin())
	registerPWAHandlers(r, root)

	// SPA shell — served publicly so unauthenticated users see the login page.
	// The Svelte app (App.svelte) checks the session internally and renders
	// LoginPage when not authenticated. /login is kept as a backward-compat alias.
	r.Get("/", serveFile(root, "index.html"))
	r.Get("/login", serveFile(root, "index.html"))

	// Public share view (unauthenticated but stricter CSP)
	r.With(PublicShareCSP).Get("/public/{token}", s.handlePublicShare())

	// Token-authenticated import endpoint for external apps (e.g. notas).
	// Disabled when PKD_IMPORT_TOKEN is not set.
	if s.cfg.ImportToken != "" {
		r.With(ImportTokenAuth(s.cfg.ImportToken)).Post("/api/import", s.handleImport())
	}

	// Static assets (unauthenticated).
	// Svelte build outputs to /assets/ with hashed filenames; legacy paths kept.
	sf := staticFileServer()
	r.Get("/assets/*", sf.ServeHTTP)
	r.Get("/icons/*", sf.ServeHTTP)
	// Legacy vanilla JS paths (still served if dist/ not yet built)
	r.Get("/css/*", sf.ServeHTTP)
	r.Get("/js/*", sf.ServeHTTP)
	r.Get("/vendor/*", sf.ServeHTTP)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(AuthRequired(s.sessions))

		// Auth
		r.Post("/api/logout", s.handleLogout())

		// Documents
		r.Post("/api/documents", s.handleCreateDocument())
		r.Get("/api/documents/{id}", s.handleGetDocument())
		r.Put("/api/documents/{id}", s.handleUpdateDocument())
		r.Delete("/api/documents/{id}", s.handleDeleteDocument())
		r.Post("/api/documents/{id}/move", s.handleMoveDocument())
		r.Post("/api/documents/{id}/reorder", s.handleReorderDocument())
		r.Post("/api/documents/{id}/restore", s.handleRestoreDocument())
		r.Post("/api/documents/{id}/favorite", s.handleToggleFavorite())
		r.Post("/api/documents/{id}/lock", s.handleToggleLock())
		r.Post("/api/documents/{id}/archive", s.handleArchiveDocument())
		r.Post("/api/documents/{id}/unarchive", s.handleUnarchiveDocument())
		r.Get("/api/documents/{id}/versions", s.handleListVersions())
		r.Get("/api/documents/{id}/versions/{vid}", s.handleGetVersion())
		r.Delete("/api/documents/{id}/versions/{vid}", s.handleDeleteVersion())
		r.Post("/api/documents/{id}/versions/{vid}/restore", s.handleRestoreVersion())
		r.Patch("/api/documents/{id}/associated-date", s.handleUpdateAssocDate())
			r.Get("/api/documents/{id}/children", s.handleListChildren())
			r.Get("/api/documents/{id}/ancestors", s.handleListAncestors())

		// Tree
			r.Get("/api/tree", s.handleTree())
			r.Post("/api/tree/sort", s.handleSortTree())

			// Bidirectional links (NEW — 003-pkm-refactor)
			r.Get("/api/documents/{id}/links", s.handleListLinks())
			r.Post("/api/documents/{id}/links", s.handleCreateLink())
			r.Delete("/api/documents/{id}/links/{linkId}", s.handleDeleteLink())

			// External URLs
			r.Get("/api/documents/{id}/urls", s.handleListURLs())
			r.Post("/api/documents/{id}/urls", s.handleCreateURL())
			r.Put("/api/documents/{id}/urls/{urlId}", s.handleUpdateURL())
			r.Delete("/api/documents/{id}/urls/{urlId}", s.handleDeleteURL())

			// Graph view (NEW — 003-pkm-refactor)
			r.Get("/api/graph", s.handleGraph())

			// External content capture (NEW — 003-pkm-refactor)
			r.Post("/api/capture", s.handleCapture())

			// Tags
			r.Get("/api/tags", s.handleListTags())
			r.Put("/api/documents/{id}/tags", s.handleSetDocumentTags())

		// Search
		r.Get("/api/search", s.handleSearch())

		// Calendar
		r.Get("/api/calendar/{year}/{month}", s.handleCalendar())

		// Attachments
		r.Get("/api/documents/{id}/attachments", s.handleListAttachments())
		r.Post("/api/documents/{id}/attachments", s.handleCreateAttachment())
		r.Post("/api/documents/{id}/attachments/from-url", s.handleCreateAttachmentFromURL())
		r.Get("/api/attachments/{id}", s.handleGetAttachment())
		r.Delete("/api/attachments/{id}", s.handleDeleteAttachment())

		// Share links (owner management)
		r.Post("/api/documents/{id}/shares", s.handleCreateShare())
		r.Delete("/api/documents/{id}/shares/{shareID}", s.handleRevokeShare())

		// Administration
		r.Get("/api/admin/trash", s.handleAdminListTrash())
		r.Delete("/api/admin/trash/{id}", s.handleAdminDeleteTrashItem())
		r.Post("/api/admin/trash/empty", s.handleAdminEmptyTrash())
		r.Post("/api/admin/backup", s.handleAdminBackup())
		r.Post("/api/admin/restore", s.handleAdminRestore())
		r.Post("/api/admin/cleanup", s.handleAdminCleanup())
		r.Get("/api/admin/tags", s.handleAdminListAllTags())
		r.Put("/api/admin/tags/rename", s.handleAdminRenameTag())
		r.Post("/api/admin/tags/prune", s.handleAdminPruneTags())
		r.Put("/api/admin/tags/{id}", s.handleAdminUpdateTag())
		r.Delete("/api/admin/tags/{id}", s.handleAdminDeleteTag())
		r.Get("/api/admin/attachments", s.handleAdminListAttachments())
		r.Get("/api/admin/attachments/orphans", s.handleAdminListOrphans())
		r.Get("/api/admin/external-images", s.handleAdminListExternalImages())
		r.Post("/api/admin/documents/{id}/import-external-images", s.handleAdminImportExternalImages())
		r.Get("/api/admin/attachments/orphans/download", s.handleAdminDownloadOrphan())
		r.Delete("/api/admin/attachments/orphans/item", s.handleAdminDeleteOrphan())
		r.Get("/api/admin/urls", s.handleAdminListURLs())
		r.Post("/api/admin/check-urls", s.handleAdminCheckURLs())
		r.Get("/api/admin/shares", s.handleAdminListShares())
		r.Delete("/api/admin/shares/{shareID}", s.handleAdminRevokeShare())

		// Server-side settings
		r.Get("/api/admin/settings", s.handleAdminGetSettings())
		r.Put("/api/admin/settings", s.handleAdminSetSettings())

		// Storage management
		r.Get("/api/admin/storage/config", s.handleAdminStorageConfig())
		r.Put("/api/admin/storage/config", s.handleAdminStorageSetBackend())
		r.Post("/api/admin/storage/test", s.handleAdminStorageTest())
		r.Post("/api/admin/storage/migrate", s.handleAdminStorageMigrate())
		r.Post("/api/admin/storage/reconcile", s.handleAdminStorageReconcile())
		r.Post("/api/admin/storage/cleanup-source", s.handleAdminStorageCleanupSource())
		r.Get("/api/admin/storage/backup-attachments", s.handleAdminStorageBackupAttachments())
		r.Post("/api/admin/storage/restore-attachments", s.handleAdminStorageRestoreAttachments())

		// Async attachment backup (005-s3-attachments-backup). Backup-start
		// dispatches a goroutine; status via jobs/{id}; download via short-lived
		// presigned URL when backend is S3.
		r.Post("/api/admin/storage/backup-start", s.handleAdminStorageBackupStart())
		r.Get("/api/admin/storage/jobs/{id}", s.handleAdminStorageGetJob())
		r.Post("/api/admin/storage/jobs/{id}/download-url", s.handleAdminStorageRegenerateDownloadURL())
		// Async restore (US2 cross-backend, US3 in-place).
		r.Post("/api/admin/storage/restore-start", s.handleAdminStorageRestoreStart())
	})

	return r
}

// serveFile returns a handler that serves a single file from an embedded FS.
// index.html is served with no-cache so browsers always fetch the latest shell,
// which in turn references hashed asset filenames from the current build.
func serveFile(root fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.ServeFileFS(w, r, root, name)
	}
}
