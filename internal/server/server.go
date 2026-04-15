package server

import (
	"database/sql"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/edalcin/pkd/internal/config"
	"github.com/edalcin/pkd/internal/sessions"
)

// Server wraps the HTTP router and its dependencies.
type Server struct {
	cfg      *config.Config
	db       *sql.DB
	sessions *sessions.Store
	handler  http.Handler
}

// New builds the chi router with all middleware and routes wired up.
func New(cfg *config.Config, db *sql.DB, sess *sessions.Store) *Server {
	s := &Server{
		cfg:      cfg,
		db:       db,
		sessions: sess,
	}
	s.handler = s.buildRouter()
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

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
	r.Get("/login", serveFile(root, "login.html"))
	r.Post("/api/login", s.handleLogin())
	registerPWAHandlers(r, root)

	// Public share view (unauthenticated but stricter CSP)
	r.With(PublicShareCSP).Get("/public/{token}", s.handlePublicShare())

	// Static assets (unauthenticated — CSS, JS, vendor, icons)
	sf := staticFileServer()
	r.Get("/css/*", sf.ServeHTTP)
	r.Get("/js/*", sf.ServeHTTP)
	r.Get("/vendor/*", sf.ServeHTTP)
	r.Get("/icons/*", sf.ServeHTTP)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(AuthRequired(s.sessions))

		// SPA shell
		r.Get("/", serveFile(root, "index.html"))

		// Auth
		r.Post("/api/logout", s.handleLogout())

		// Documents
		r.Post("/api/documents", s.handleCreateDocument())
		r.Get("/api/documents/{id}", s.handleGetDocument())
		r.Put("/api/documents/{id}", s.handleUpdateDocument())
		r.Delete("/api/documents/{id}", s.handleDeleteDocument())
		r.Post("/api/documents/{id}/move", s.handleMoveDocument())
		r.Post("/api/documents/{id}/restore", s.handleRestoreDocument())

		// Tree
		r.Get("/api/tree", s.handleTree())

		// Tags
		r.Get("/api/tags", s.handleListTags())
		r.Put("/api/documents/{id}/tags", s.handleSetDocumentTags())

		// Search
		r.Get("/api/search", s.handleSearch())

		// Calendar
		r.Get("/api/calendar/{year}/{month}", s.handleCalendar())

		// Attachments
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
		r.Put("/api/admin/tags/rename", s.handleAdminRenameTag())
	})

	return r
}

// serveFile returns a handler that serves a single file from an embedded FS.
func serveFile(root fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, root, name)
	}
}
