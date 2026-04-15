package model

import "time"

// Attachment mirrors the attachments table and the OpenAPI Attachment schema.
type Attachment struct {
	ID             int64     `json:"id"`
	DocumentID     int64     `json:"document_id"`
	OriginalName   string    `json:"original_name"`
	StoredFilename string    `json:"-"` // never sent to clients
	MimeType       string    `json:"mime_type"`
	SizeBytes      int64     `json:"size_bytes"`
	URL            string    `json:"url"` // /api/attachments/{id}
	CreatedAt      time.Time `json:"created_at"`
}
