package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/edalcin/pkd/internal/model"
)

// ErrConflict is returned by CreateLink when the link already exists.
var ErrConflict = fmt.Errorf("link already exists")

// LinkStore provides CRUD for document_links and graph query operations.
type LinkStore struct {
	db *sql.DB
}

// NewLinkStore wraps db.
func NewLinkStore(db *sql.DB) *LinkStore {
	return &LinkStore{db: db}
}

// GetLinksForDocument returns all documents related to docID, regardless of
// which side holds source_id vs target_id. The result is deduplicated so that
// a pair that has links in both directions (A→B and B→A) appears only once.
func (s *LinkStore) GetLinksForDocument(docID int64) (*model.LinksResponse, error) {
	const q = `
		SELECT
			dl.id,
			CASE WHEN dl.source_id = ? THEN dl.target_id ELSE dl.source_id END AS related_id,
			CASE WHEN dl.source_id = ? THEN td.title    ELSE sd.title    END AS related_title,
			CASE WHEN dl.source_id = ?
			     THEN (td.trashed_at IS NOT NULL)
			     ELSE (sd.trashed_at IS NOT NULL) END    AS related_trashed,
			dl.created_at
		FROM document_links dl
		JOIN documents sd ON sd.id = dl.source_id
		JOIN documents td ON td.id = dl.target_id
		WHERE dl.source_id = ? OR dl.target_id = ?
		ORDER BY related_title`

	rows, err := s.db.Query(q, docID, docID, docID, docID, docID)
	if err != nil {
		return nil, fmt.Errorf("links.GetLinksForDocument: %w", err)
	}
	defer rows.Close()

	seen := map[int64]bool{}
	var related []model.RelatedLink
	for rows.Next() {
		var e model.RelatedLink
		var createdAt string
		var relatedTrashed bool
		if err := rows.Scan(&e.ID, &e.RelatedID, &e.RelatedTitle, &relatedTrashed, &createdAt); err != nil {
			return nil, err
		}
		e.RelatedTrashed = relatedTrashed
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if !seen[e.RelatedID] {
			seen[e.RelatedID] = true
			related = append(related, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if related == nil {
		related = []model.RelatedLink{}
	}
	return &model.LinksResponse{Related: related}, nil
}

// CreateLink inserts a new link from sourceID to targetID. Returns ErrConflict
// if the link already exists, or ErrNotFound if either document is missing.
func (s *LinkStore) CreateLink(sourceID, targetID int64) (*model.RelatedLink, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(
		`INSERT INTO document_links (source_id, target_id, manual, created_at) VALUES (?, ?, 1, ?)`,
		sourceID, targetID, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("links.CreateLink: %w", err)
	}

	resp, err := s.GetLinksForDocument(sourceID)
	if err != nil {
		return nil, err
	}
	for i := range resp.Related {
		if resp.Related[i].RelatedID == targetID {
			return &resp.Related[i], nil
		}
	}
	return &model.RelatedLink{RelatedID: targetID}, nil
}

// DeleteLink removes a link by its id. Returns ErrNotFound if not found.
func (s *LinkStore) DeleteLink(id int64) error {
	res, err := s.db.Exec(`DELETE FROM document_links WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("links.DeleteLink: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetGraphData returns nodes and edges for the graph visualisation.
// When includeAll is false, only documents with at least one link are included.
// When tags is non-empty, only documents with ALL of those tags are included.
// Tag nodes are always appended (negative IDs) for every tag referenced by
// the included document nodes.
func (s *LinkStore) GetGraphData(tags []string, includeAll bool) (*model.GraphData, error) {
	edges, err := s.queryAllEdges()
	if err != nil {
		return nil, err
	}

	connectedIDs := map[int64]bool{}
	for _, e := range edges {
		connectedIDs[e.Source] = true
		connectedIDs[e.Target] = true
	}
	// Docs with tags are also "connected" — they'll have doc→tag edges.
	taggedRows, err := s.db.Query(`SELECT DISTINCT document_id FROM document_tags`)
	if err != nil {
		return nil, fmt.Errorf("links.GetGraphData tagged docs: %w", err)
	}
	for taggedRows.Next() {
		var id int64
		if taggedRows.Scan(&id) == nil {
			connectedIDs[id] = true
		}
	}
	taggedRows.Close()

	var nodeRows *sql.Rows
	if len(tags) > 0 {
		ph := strings.Repeat("?,", len(tags))
		ph = ph[:len(ph)-1]
		q := fmt.Sprintf(`
			SELECT d.id, d.title, COALESCE(d.icon,'')
			FROM documents d
			WHERE d.trashed_at IS NULL
			  AND (SELECT COUNT(DISTINCT t.name)
			       FROM tags t JOIN document_tags dt ON dt.tag_id = t.id
			       WHERE dt.document_id = d.id AND t.name IN (%s)) = ?
			ORDER BY d.title`, ph)
		args := make([]interface{}, 0, len(tags)+1)
		for _, t := range tags {
			args = append(args, t)
		}
		args = append(args, len(tags))
		nodeRows, err = s.db.Query(q, args...)
	} else {
		nodeRows, err = s.db.Query(`
			SELECT d.id, d.title, COALESCE(d.icon,'')
			FROM documents d
			WHERE d.trashed_at IS NULL
			ORDER BY d.title`)
	}
	if err != nil {
		return nil, fmt.Errorf("links.GetGraphData nodes: %w", err)
	}
	defer nodeRows.Close()

	var nodes []model.GraphNode
	for nodeRows.Next() {
		var n model.GraphNode
		if err := nodeRows.Scan(&n.ID, &n.Title, &n.Icon); err != nil {
			return nil, fmt.Errorf("links.GetGraphData scan: %w", err)
		}
		n.NodeType = "doc"
		if includeAll || connectedIDs[n.ID] {
			nodes = append(nodes, n)
		}
	}
	if err := nodeRows.Err(); err != nil {
		return nil, fmt.Errorf("links.GetGraphData rows: %w", err)
	}

	for i := range nodes {
		nodes[i].Tags, _ = s.queryTagsForDoc(nodes[i].ID)
	}

	nodeSet := map[int64]bool{}
	for _, n := range nodes {
		nodeSet[n.ID] = true
	}
	var filteredEdges []model.GraphEdge
	for _, e := range edges {
		if nodeSet[e.Source] && nodeSet[e.Target] {
			filteredEdges = append(filteredEdges, e)
		}
	}

	// Add tag nodes (negative IDs) and doc→tag edges for every tag
	// that appears on at least one included document node.
	tagNodeMap := map[int64]model.GraphNode{} // tag DB id → GraphNode
	for _, n := range nodes {
		for _, tagName := range n.Tags {
			var tagID int64
			if err := s.db.QueryRow(`SELECT id FROM tags WHERE name = ? COLLATE NOCASE`, tagName).Scan(&tagID); err != nil {
				continue
			}
			graphTagID := -tagID
			if _, exists := tagNodeMap[tagID]; !exists {
				tagNodeMap[tagID] = model.GraphNode{
					ID:       graphTagID,
					Title:    "#" + tagName,
					NodeType: "tag",
				}
			}
			filteredEdges = append(filteredEdges, model.GraphEdge{Source: n.ID, Target: graphTagID})
		}
	}
	for _, tn := range tagNodeMap {
		nodes = append(nodes, tn)
	}

	return &model.GraphData{
		Nodes: nonNilNodes(nodes),
		Edges: nonNilEdges(filteredEdges),
	}, nil
}

// SyncLinksFromHTML parses data-doc-link attributes from an HTML document body,
// diffs against current document_links rows, and inserts/deletes as needed.
// Must be called inside the same transaction as the document save.
func (s *LinkStore) SyncLinksFromHTML(tx *sql.Tx, docID int64, bodyHTML string) error {
	targetIDs := extractDocLinkIDs(bodyHTML)

	// Load current auto-managed links from DB (manual=1 links are never touched by sync)
	rows, err := tx.Query(`SELECT id, target_id FROM document_links WHERE source_id = ? AND manual = 0`, docID)
	if err != nil {
		return fmt.Errorf("links.Sync query: %w", err)
	}
	current := map[int64]int64{} // target_id → link id
	for rows.Next() {
		var linkID, targetID int64
		if err := rows.Scan(&linkID, &targetID); err != nil {
			rows.Close()
			return err
		}
		current[targetID] = linkID
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Insert new links (those in body but not in DB)
	for _, tid := range targetIDs {
		if _, exists := current[tid]; !exists {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO document_links (source_id, target_id, created_at) VALUES (?, ?, ?)`,
				docID, tid, now,
			); err != nil {
				return fmt.Errorf("links.Sync insert: %w", err)
			}
		}
		delete(current, tid) // mark as still present
	}

	// Delete stale links (in DB but no longer in body)
	for _, staleID := range current {
		if _, err := tx.Exec(`DELETE FROM document_links WHERE id = ?`, staleID); err != nil {
			return fmt.Errorf("links.Sync delete: %w", err)
		}
	}
	return nil
}

// extractDocLinkIDs tokenizes an HTML string and returns all unique document IDs
// referenced via data-doc-link attributes (e.g. <span data-doc-link="42">).
func extractDocLinkIDs(bodyHTML string) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	z := html.NewTokenizer(strings.NewReader(bodyHTML))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		for _, attr := range tok.Attr {
			if attr.Key == "data-doc-link" {
				id, err := strconv.ParseInt(strings.TrimSpace(attr.Val), 10, 64)
				if err == nil && id > 0 && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

// ── private helpers ──────────────────────────────────────────────────────────

func (s *LinkStore) queryAllEdges() ([]model.GraphEdge, error) {
	rows, err := s.db.Query(`
		SELECT dl.source_id, dl.target_id
		FROM document_links dl
		JOIN documents sd ON sd.id = dl.source_id AND sd.trashed_at IS NULL
		JOIN documents td ON td.id = dl.target_id AND td.trashed_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("links.queryAllEdges: %w", err)
	}
	defer rows.Close()
	var edges []model.GraphEdge
	for rows.Next() {
		var e model.GraphEdge
		if err := rows.Scan(&e.Source, &e.Target); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func (s *LinkStore) queryTagsForDoc(docID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT t.name FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		WHERE dt.document_id = ? ORDER BY t.name`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

func nonNilNodes(nodes []model.GraphNode) []model.GraphNode {
	if nodes == nil {
		return []model.GraphNode{}
	}
	return nodes
}

func nonNilEdges(edges []model.GraphEdge) []model.GraphEdge {
	if edges == nil {
		return []model.GraphEdge{}
	}
	return edges
}
