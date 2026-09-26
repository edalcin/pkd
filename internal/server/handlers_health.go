package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type healthHandler struct {
	db *sql.DB
}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Scan, not Row.Err: only Scan releases the connection, and the pool has one.
	var one int
	if err := h.db.QueryRow("SELECT 1").Scan(&one); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
