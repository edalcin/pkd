package model

import "time"

// Link mirrors the document_links table.
type Link struct {
	ID        int64     `json:"id"`
	SourceID  int64     `json:"source_id"`
	TargetID  int64     `json:"target_id"`
	CreatedAt time.Time `json:"created_at"`
}

// LinkEntry is the enriched response shape returned by GET /api/documents/{id}/links.
// It includes the titles of both documents so the client does not need to make
// additional requests to display the link list.
type LinkEntry struct {
	ID            int64     `json:"id"`
	SourceID      int64     `json:"source_id"`
	TargetID      int64     `json:"target_id"`
	SourceTitle   string    `json:"source_title"`
	TargetTitle   string    `json:"target_title"`
	TargetTrashed bool      `json:"target_trashed"`
	CreatedAt     time.Time `json:"created_at"`
}

// LinksResponse is returned by GET /api/documents/{id}/links.
type LinksResponse struct {
	Outgoing []LinkEntry `json:"outgoing"`
	Incoming []LinkEntry `json:"incoming"`
}

// GraphNode is a node in the graph response.
type GraphNode struct {
	ID    int64    `json:"id"`
	Title string   `json:"title"`
	Icon  string   `json:"icon,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

// GraphEdge is a directed edge in the graph response.
type GraphEdge struct {
	Source int64 `json:"source"`
	Target int64 `json:"target"`
}

// GraphData is returned by GET /api/graph.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}
