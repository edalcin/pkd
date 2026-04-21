package server

import (
	"net/http"

	"github.com/edalcin/pkd/internal/model"
)

func (s *Server) handleTree() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tagFilter := r.URL.Query()["tag"]
		favoriteOnly := r.URL.Query().Get("favorite") == "1"
		q := r.URL.Query().Get("q")

		docs, err := s.docs.ListTree(tagFilter, favoriteOnly, q)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		tree := buildTree(docs)
		writeJSON(w, http.StatusOK, tree)
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
