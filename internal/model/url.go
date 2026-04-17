package model

import "time"

// DocumentURL is an external URL associated with a document.
type DocumentURL struct {
	ID         int64     `json:"id"`
	DocumentID int64     `json:"document_id"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
}
