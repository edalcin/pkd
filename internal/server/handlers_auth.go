package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

const sessionCookieMaxAge = 86400 * 30 // 30 days

func (s *Server) handleLogin() http.HandlerFunc {
	type request struct {
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !security.VerifyMaster(req.Password, s.cfg.Password) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ip := r.RemoteAddr
		sess := s.sessions.Create(ip)
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    sess.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, // enforced by reverse proxy in prod
			SameSite: http.SameSiteStrictMode,
			Expires:  time.Now().Add(sessionCookieMaxAge * time.Second),
			MaxAge:   sessionCookieMaxAge,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			s.sessions.Delete(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}
