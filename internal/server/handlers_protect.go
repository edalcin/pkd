package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/store"
)

// handleProtectDocument encrypts a document's body at rest (POST /api/documents/{id}/protect).
func (s *Server) handleProtectDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.emailEnabled {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "email 2FA not configured"})
			return
		}
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.GetByID(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if doc.Encrypted {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already encrypted"})
			return
		}
		key := security.DeriveDocKey(s.cfg.Password)
		cipherHTML, err := security.EncryptDoc(doc.BodyHTML, key)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out, err := s.docs.Protect(id, cipherHTML)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sess := SessionFromContext(r.Context())
		if sess != nil {
			s.sessions.UnlockDoc(sess.ID, id)
		}
		out.BodyHTML = doc.BodyHTML
		out.Encrypted = true
		s.embedder.notify()
		writeJSON(w, http.StatusOK, out)
	}
}

// handleUnprotectDocument decrypts a document back to plaintext (POST /api/documents/{id}/unprotect).
// Only allowed inside a session that has already unlocked the document.
func (s *Server) handleUnprotectDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.GetByID(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !doc.Encrypted {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "not encrypted"})
			return
		}
		sess := SessionFromContext(r.Context())
		if sess == nil || !s.sessions.IsDocUnlocked(sess.ID, id) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unlock required"})
			return
		}
		key := security.DeriveDocKey(s.cfg.Password)
		plain, err := security.DecryptDoc(doc.BodyHTML, key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decrypt failed — master password changed?"})
			return
		}
		plainText := security.ExtractPlainText(plain)
		out, err := s.docs.Unprotect(id, plain, plainText)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		s.embedder.notify()
		writeJSON(w, http.StatusOK, out)
	}
}

// handleRequestDocCode e-mails a fresh 6-digit code to open a protected
// document (POST /api/documents/{id}/unlock/request).
func (s *Server) handleRequestDocCode() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.emailEnabled {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "email 2FA not configured"})
			return
		}
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		doc, err := s.docs.GetByID(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !doc.Encrypted {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not encrypted"})
			return
		}
		sess := SessionFromContext(r.Context())
		if sess != nil && s.sessions.IsDocUnlocked(sess.ID, id) {
			w.WriteHeader(http.StatusNoContent) // already open, no e-mail needed
			return
		}
		sessID := ""
		if sess != nil {
			sessID = sess.ID
		}
		code := security.NewNumericCode(codeDigits)
		challengeID := s.challenges.create("doc", code, sessID, id)
		if err := s.send2FACode("PKD — Código para documento protegido",
			"Código para abrir \""+doc.Title+"\": "+code+"\n\nExpira em 10 minutos."); err != nil {
			http.Error(w, "email send failed", http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"challenge_id": challengeID})
	}
}

// handleUnlockDocument verifies the e-mailed code and returns the decrypted
// body for the rest of the browser session (POST /api/documents/{id}/unlock).
func (s *Server) handleUnlockDocument() http.HandlerFunc {
	type request struct {
		ChallengeID string `json:"challenge_id"`
		Code        string `json:"code"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChallengeID == "" || req.Code == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		sess := SessionFromContext(r.Context())
		if sess == nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		ch, ok := s.challenges.verify(req.ChallengeID, req.Code)
		if !ok || ch.kind != "doc" || ch.docID != id || ch.sessionID != sess.ID {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.sessions.UnlockDoc(sess.ID, id)
		doc, err := s.docs.GetByID(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		key := security.DeriveDocKey(s.cfg.Password)
		plain, err := security.DecryptDoc(doc.BodyHTML, key)
		if err != nil {
			http.Error(w, "decrypt failed", http.StatusInternalServerError)
			return
		}
		doc.BodyHTML = plain // Encrypted stays true — body is decrypted for display only
		writeJSON(w, http.StatusOK, doc)
	}
}
