package server

import (
	"net/http"

	"github.com/edalcin/pkd/internal/model"
)

func (s *Server) handleTree() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view := r.URL.Query().Get("view") // "active" | "archived" | "all" — empty defaults to "active"
		tagFilter := r.URL.Query()["tag"]
		favoriteOnly := r.URL.Query().Get("favorite") == "1"
		q := r.URL.Query().Get("q")
		mode := r.URL.Query().Get("mode")

		var docs []*model.Document
		var err error
		if mode == "semantic" && q != "" {
			if s.cfg.GeminiAPIKey == "" {
				http.Error(w, `{"error":"GEMINI_API_KEY not configured"}`, http.StatusServiceUnavailable)
				return
			}
			hits, e := s.links.SemanticSearchDocIDs(r.Context(), s.cfg.GeminiAPIKey, q)
			if e != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			ids := make([]int64, len(hits))
			for i, h := range hits {
				ids[i] = h.DocID
			}
			docsByID := make(map[int64]*model.Document, len(hits))
			if fetched, fe := s.docs.ListByIDs(ids); fe != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			} else {
				for _, d := range fetched {
					docsByID[d.ID] = d
				}
			}
			// Build flat list preserving score order.
			flat := make([]*model.DocumentTreeNode, 0, len(hits))
			for _, h := range hits {
				d, ok := docsByID[h.DocID]
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
					Archived:   d.Archived,
					ArchivedAt: d.ArchivedAt,
					Tags:       d.Tags,
					Children:   []*model.DocumentTreeNode{},
					Score:      float64(h.Score),
				})
			}
			writeJSON(w, http.StatusOK, flat)
			return
		}
		docs, err = s.docs.ListTree(view, tagFilter, favoriteOnly, q)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, buildTree(docs))
	}
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
