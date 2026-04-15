package server

import "net/http"

func (s *Server) handleAdminListTrash() http.HandlerFunc {
	return stub()
}
func (s *Server) handleAdminDeleteTrashItem() http.HandlerFunc {
	return stub()
}
func (s *Server) handleAdminEmptyTrash() http.HandlerFunc {
	return stub()
}
func (s *Server) handleAdminBackup() http.HandlerFunc {
	return stub()
}
func (s *Server) handleAdminRestore() http.HandlerFunc {
	return stub()
}
func (s *Server) handleAdminCleanup() http.HandlerFunc {
	return stub()
}
func (s *Server) handleAdminRenameTag() http.HandlerFunc {
	return stub()
}

func stub() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}
