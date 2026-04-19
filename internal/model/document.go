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
	Tags             []string   `json:"tags,omitempty"`
	AttachmentIDs    []int64    `json:"attachment_ids,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
	Tags       []string            `json:"tags,omitempty"`
	Children   []*DocumentTreeNode `json:"children"`
}

// VersionConflict is returned as the body of a 409 response when a document
// save is rejected because the client's version is stale.
type VersionConflict struct {
	StoredVersion int64     `json:"stored_version"`
	Stored        *Document `json:"stored"`
}
