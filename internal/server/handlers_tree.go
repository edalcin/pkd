package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/edalcin/pkd/internal/model"
	"github.com/edalcin/pkd/internal/store"
)

// hybridResultLimit caps the number of RRF-fused results returned by a
// text search. Each leg (lexical, semantic) is separately capped upstream.
const hybridResultLimit = 100

func (s *Server) handleTree() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view := r.URL.Query().Get("view") // "active" | "archived" | "all" — empty defaults to "active"
		tagFilter := r.URL.Query()["tag"]
		favoriteOnly := r.URL.Query().Get("favorite") == "1"
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		if q != "" {
			s.respondHybridSearch(w, r, q, view, tagFilter, favoriteOnly)
			return
		}
		docs, err := s.docs.ListTree(view, tagFilter, favoriteOnly)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, buildTree(docs))
	}
}

// respondHybridSearch runs the lexical and semantic retrievers for q, fuses
// their rankings by Reciprocal Rank Fusion, and writes a flat (non-tree)
// list of matching documents ordered by fused rank. The semantic leg
// degrades to empty — never an error response — when GEMINI_API_KEY is
// unset or the Gemini call fails, so the result always falls back to the
// lexical ranking.
func (s *Server) respondHybridSearch(w http.ResponseWriter, r *http.Request, q, view string, tagFilter []string, favoriteOnly bool) {
	lex, err := s.search.LexicalDocIDs(q)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	hits, err := s.links.SemanticSearchDocIDs(r.Context(), s.cfg.GeminiAPIKey, q)
	if err != nil {
		log.Printf("hybrid search: semantic leg failed: %v", err)
		hits = nil
	}
	semIDs := make([]int64, len(hits))
	for i, h := range hits {
		semIDs[i] = h.DocID
	}

	fused := store.FuseRRF(lex, semIDs, hybridResultLimit)
	if len(fused) == 0 {
		writeJSON(w, http.StatusOK, []*model.DocumentTreeNode{})
		return
	}

	docs, err := s.docs.ListByIDsFiltered(fused, view, tagFilter, favoriteOnly)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	docsByID := make(map[int64]*model.Document, len(docs))
	for _, d := range docs {
		docsByID[d.ID] = d
	}

	flat := make([]*model.DocumentTreeNode, 0, len(fused))
	for _, id := range fused {
		d, ok := docsByID[id]
		if !ok {
			continue
		}
		flat = append(flat, &model.DocumentTreeNode{
			ID:         d.ID,
			ParentID:   d.ParentID,
			Title:      d.Title,
			Icon:       d.Icon,
			Position:   d.Position,
			Version:    d.Version,
			IsFavorite: d.IsFavorite,
			Locked:     d.Locked,
			Encrypted:  d.Encrypted,
			Archived:   d.Archived,
			ArchivedAt: d.ArchivedAt,
			Tags:       d.Tags,
			Children:   []*model.DocumentTreeNode{},
		})
	}
	writeJSON(w, http.StatusOK, flat)
}

// buildTree converts a flat list of documents into a nested tree.
// Documents without a parent (or whose parent is not in the list) become roots.
func buildTree(docs []*model.Document) []*model.DocumentTreeNode {
	nodes := make(map[int64]*model.DocumentTreeNode, len(docs))
	for _, d := range docs {
		nodes[d.ID] = &model.DocumentTreeNode{
			ID:         d.ID,
			ParentID:   d.ParentID,
			Title:      d.Title,
			Icon:       d.Icon,
			Position:   d.Position,
			Version:    d.Version,
			IsFavorite: d.IsFavorite,
			Locked:     d.Locked,
			Encrypted:  d.Encrypted,
			Archived:   d.Archived,
			ArchivedAt: d.ArchivedAt,
			Tags:       d.Tags,
			Children:   []*model.DocumentTreeNode{},
		}
	}

	var roots []*model.DocumentTreeNode
	for _, d := range docs {
		node := nodes[d.ID]
		if d.ParentID == nil {
			roots = append(roots, node)
		} else if parent, ok := nodes[*d.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		} else {
			// Parent not in result set (e.g. tag-filtered tree) — treat as root
			roots = append(roots, node)
		}
	}

	if roots == nil {
		roots = []*model.DocumentTreeNode{}
	}
	return roots
}
