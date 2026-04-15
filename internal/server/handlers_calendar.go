package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type calendarDay struct {
	Day       int `json:"day"`
	Documents []calendarDoc `json:"documents"`
}

type calendarDoc struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
}

func (s *Server) handleCalendar() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		year, err := strconv.Atoi(chi.URLParam(r, "year"))
		if err != nil || year < 1 || year > 9999 {
			http.Error(w, "invalid year", http.StatusBadRequest)
			return
		}
		month, err := strconv.Atoi(chi.URLParam(r, "month"))
		if err != nil || month < 1 || month > 12 {
			http.Error(w, "invalid month", http.StatusBadRequest)
			return
		}

		yearMonth := fmt.Sprintf("%04d-%02d", year, month)
		rows, err := s.db.Query(`
			SELECT id, title, icon, CAST(strftime('%d', created_at) AS INTEGER) AS day
			FROM documents
			WHERE strftime('%Y-%m', created_at) = ? AND trashed_at IS NULL
			ORDER BY created_at`, yearMonth)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		days := make(map[int]*calendarDay)
		for rows.Next() {
			var id int64
			var title, icon string
			var day int
			var iconNull *string
			if err := rows.Scan(&id, &title, &iconNull, &day); err != nil {
				continue
			}
			if iconNull != nil {
				icon = *iconNull
			}
			if _, ok := days[day]; !ok {
				days[day] = &calendarDay{Day: day}
			}
			days[day].Documents = append(days[day].Documents, calendarDoc{ID: id, Title: title, Icon: icon})
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		result := make([]*calendarDay, 0, len(days))
		for d := 1; d <= 31; d++ {
			if day, ok := days[d]; ok {
				result = append(result, day)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
