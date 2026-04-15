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
