package model

import "time"

// ShareLink mirrors the share_links table.
type ShareLink struct {
	ID         int64      `json:"id"`
	DocumentID int64      `json:"document_id"`
	TokenHash  []byte     `json:"-"` // never sent to clients
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// ShareCreateResponse is returned once when a share link is created.
// The plaintext token is shown exactly once and never stored.
type ShareCreateResponse struct {
	Token    string `json:"token"`
	URL      string `json:"url"`
	RevokeID int64  `json:"revoke_id"`
}

// ShareWithDoc is returned by the admin list-shares endpoint.
// It enriches a ShareLink with the associated document title and the full public URL.
type ShareWithDoc struct {
	ID            int64     `json:"id"`
	DocumentID    int64     `json:"document_id"`
	DocumentTitle string    `json:"document_title"`
	CreatedAt     time.Time `json:"created_at"`
	Token         string    `json:"token"`          // plaintext token; empty for shares created before this field was added
	URL           string    `json:"url,omitempty"`  // full public URL, populated by the handler
}
