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

// AdminURL is a flattened view of a document URL including the document title.
type AdminURL struct {
	ID            int64  `json:"id"`
	DocumentID    int64  `json:"document_id"`
	DocumentTitle string `json:"document_title"`
	URL           string `json:"url"`
	Title         string `json:"title"`
}
