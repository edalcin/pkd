package server

import (
	"encoding/json"
	"log"
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
		// Throttle check — 5 failures → 30-minute lockout (FR-002, clarification Q5→C)
		if !s.throttle.Allow(r) {
			ThrottleHeader(w, s.throttle.RetryAfter(r))
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if !security.VerifyMaster(req.Password, s.cfg.Password) {
			s.throttle.RecordFailure(r)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		s.throttle.RecordSuccess(r)

		// Direct login when 2FA disabled OR device already trusted.
		if !s.emailEnabled || s.deviceTrusted(r) {
			s.startSession(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Otherwise: e-mail a code and require /api/login/2fa.
		code := security.NewNumericCode(codeDigits)
		id := s.challenges.create("login", code, "", 0)
		if err := s.send2FACode("PKD — Código de acesso", "Seu código de acesso é: "+code+"\n\nExpira em 10 minutos."); err != nil {
			log.Printf("2fa login e-mail send failed: %v", err)
			writeJSON(w, http.StatusOK, map[string]any{"two_factor_required": true, "challenge_id": id, "email_failed": true})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"two_factor_required": true, "challenge_id": id})
	}
}

// startSession creates a session and sets the pkd_session cookie (shared by
// the direct-login and post-2FA login paths).
func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	sess := s.sessions.Create(r.RemoteAddr)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // enforced by reverse proxy in production
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(sessionCookieMaxAge * time.Second),
		MaxAge:   sessionCookieMaxAge,
	})
}

// deviceTrusted reports whether the request carries a known trusted-device cookie.
func (s *Server) deviceTrusted(r *http.Request) bool {
	ck, err := r.Cookie(deviceCookieName)
	if err != nil || ck.Value == "" {
		return false
	}
	ok, _ := s.devices.IsTrusted(security.HashSHA256(ck.Value))
	return ok
}

// handleLogin2FA verifies the e-mailed code, trusts the device permanently,
// and starts the session — completing the login started by handleLogin.
func (s *Server) handleLogin2FA() http.HandlerFunc {
	type request struct {
		ChallengeID string `json:"challenge_id"`
		Code        string `json:"code"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChallengeID == "" || req.Code == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ch, ok := s.challenges.verifyFunc(req.ChallengeID, func(ch *challenge) bool {
			if security.ConstantTimeEqualBytes(ch.codeHash, security.HashSHA256(req.Code)) {
				return true
			}
			used, _ := s.backupCodes.Consume(req.Code)
			return used
		})
		if !ok || ch.kind != "login" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Trust this device forever.
		token := security.NewToken(32)
		_ = s.devices.Trust(security.HashSHA256(token), r.UserAgent())
		http.SetCookie(w, &http.Cookie{
			Name:     deviceCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   deviceCookieMaxAge,
		})
		s.startSession(w, r)
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
