package server

import (
	"database/sql"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/edalcin/pkd/internal/config"
	"github.com/edalcin/pkd/internal/sessions"
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
	throttle    *Throttle
	handler     http.Handler
}

// New builds the chi router with all middleware and routes wired up.
func New(cfg *config.Config, db *sql.DB, sess *sessions.Store) *Server {
	s := &Server{
		cfg:         cfg,
		db:          db,
		sessions:    sess,
		docs:        store.NewDocumentStore(db),
		attachments: store.NewAttachmentStore(db, cfg.AttachmentsPath),
		tags:        store.NewTagStore(db),
		search:      store.NewSearchStore(db),
		shares:      store.NewShareStore(db),
		backup:      store.NewBackupStore(db, cfg.DBPath),
		links:       store.NewLinkStore(db),
		urls:        store.NewURLStore(db),
		throttle:    NewThrottle(cfg.TrustProxyHeaders),
	}
	s.handler = s.buildRouter()
	return s
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
		r.Post("/api/documents/{id}/restore", s.handleRestoreDocument())
			r.Get("/api/documents/{id}/children", s.handleListChildren())

		// Tree
			r.Get("/api/tree", s.handleTree())

			// Bidirectional links (NEW — 003-pkm-refactor)
			r.Get("/api/documents/{id}/links", s.handleListLinks())
			r.Post("/api/documents/{id}/links", s.handleCreateLink())
			r.Delete("/api/documents/{id}/links/{linkId}", s.handleDeleteLink())

			// External URLs
			r.Get("/api/documents/{id}/urls", s.handleListURLs())
			r.Post("/api/documents/{id}/urls", s.handleCreateURL())
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
		r.Post("/api/admin/check-urls", s.handleAdminCheckURLs())
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
