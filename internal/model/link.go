package model

import "time"

// Link mirrors the document_links table.
type Link struct {
	ID        int64     `json:"id"`
	SourceID  int64     `json:"source_id"`
	TargetID  int64     `json:"target_id"`
	CreatedAt time.Time `json:"created_at"`
}

// RelatedLink is the enriched shape used by GET /api/documents/{id}/links.
// It describes the relationship from the perspective of the requesting document:
// RelatedID/RelatedTitle refer to the *other* document regardless of which
// side stored the original source_id/target_id.
type RelatedLink struct {
	ID             int64     `json:"id"`
	RelatedID      int64     `json:"related_id"`
	RelatedTitle   string    `json:"related_title"`
	RelatedTrashed bool      `json:"related_trashed"`
	CreatedAt      time.Time `json:"created_at"`
}

// LinksResponse is returned by GET /api/documents/{id}/links.
type LinksResponse struct {
	Related []RelatedLink `json:"related"`
}

// GraphNode is a node in the graph response.
type GraphNode struct {
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	Icon     string   `json:"icon,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	NodeType string   `json:"node_type"` // "doc" or "tag"
}

// GraphEdge is a directed edge in the graph response.
type GraphEdge struct {
	Source   int64  `json:"source"`
	Target   int64  `json:"target"`
	EdgeType string `json:"edge_type,omitempty"`
}

// GraphData is returned by GET /api/graph.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}
