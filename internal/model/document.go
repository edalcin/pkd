package model

import "time"

// Document mirrors the documents table and the OpenAPI Document schema.
type Document struct {
	ID               int64      `json:"id"`
	ParentID         *int64     `json:"parent_id"`
	Title            string     `json:"title"`
	BodyHTML         string     `json:"body_html"`
	BodyText         string     `json:"body_text,omitempty"`
	Icon             string     `json:"icon,omitempty"`
	Position         int        `json:"position"`
	Version          int64      `json:"version"`
	TrashedAt        *time.Time `json:"trashed_at,omitempty"`
	OriginalParentID *int64     `json:"original_parent_id,omitempty"`
	IsFavorite       bool       `json:"is_favorite"`
	Locked           bool       `json:"locked"`
	Archived         bool       `json:"archived"`
	ArchivedAt       *time.Time `json:"archived_at"`
	Tags             []string   `json:"tags,omitempty"`
	AttachmentIDs    []int64    `json:"attachment_ids,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	AssocYear        *int       `json:"assoc_year,omitempty"`
	AssocMonth       *int       `json:"assoc_month,omitempty"`
	AssocDay         *int       `json:"assoc_day,omitempty"`
}

// DocumentTreeNode is the nested shape returned by GET /api/tree.
type DocumentTreeNode struct {
	ID       int64               `json:"id"`
	ParentID *int64              `json:"parent_id"`
	Title    string              `json:"title"`
	Icon     string              `json:"icon,omitempty"`
	Position int                 `json:"position"`
	Version  int64               `json:"version"`
	IsFavorite bool                `json:"is_favorite"`
	Locked     bool                `json:"locked"`
	Archived   bool                `json:"archived"`
	ArchivedAt *time.Time          `json:"archived_at"`
	Tags       []string            `json:"tags,omitempty"`
	Children   []*DocumentTreeNode `json:"children"`
}

// VersionConflict is returned as the body of a 409 response when a document
// save is rejected because the client's version is stale.
type VersionConflict struct {
	ConflictType  string    `json:"conflict_type"` // "version"
	StoredVersion int64     `json:"stored_version"`
	Stored        *Document `json:"stored"`
}

// TitleConflict is returned as the body of a 409 response when a document
// save is rejected because another document already holds the same title.
type TitleConflict struct {
	ConflictType string    `json:"conflict_type"` // "title"
	ExistingDoc  *Document `json:"existing_doc"`
	Suggestions  []string  `json:"suggestions"`
}

// Ancestor is a minimal document node used for breadcrumb display.
type Ancestor struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
}

// DocumentVersion is a content snapshot of a document at a given point in time.
// BodyHTML is omitted from list responses (fetched individually via GetVersion).
type DocumentVersion struct {
	ID            int64     `json:"id"`
	DocumentID    int64     `json:"document_id"`
	DocVersion    int64     `json:"doc_version"`
	Title         string    `json:"title"`
	BodyHTML      string    `json:"body_html,omitempty"`
	Icon          string    `json:"icon,omitempty"`
	ContentSHA256 string    `json:"content_sha256"`
	CreatedAt     time.Time `json:"created_at"`
}
