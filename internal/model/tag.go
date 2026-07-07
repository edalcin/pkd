package model

import "time"

// Tag mirrors the tags table.
type Tag struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

// TagWithCount is returned by GET /api/tags.
type TagWithCount struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

// TagDocStats is per-tag document counts split by archived status, used by
// the admin dashboard "documents by tag" card.
type TagDocStats struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Active   int64  `json:"active"`
	Archived int64  `json:"archived"`
}
