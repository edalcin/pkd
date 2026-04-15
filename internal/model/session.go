package model

import "time"

// Session represents an authenticated session stored in memory only.
// Sessions are never persisted to the database and are lost on server restart.
type Session struct {
	ID         string
	IP         string
	CreatedAt  time.Time
	LastSeenAt time.Time
}
