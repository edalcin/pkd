package store

import (
	"testing"
)

// TestEmbeddableWhere pins the selection rule shared by the embedding sweep and
// its prune step: archived documents are embeddable (so they reach semantic
// search), trashed and encrypted ones are not.
func TestEmbeddableWhere(t *testing.T) {
	db, err := Open("file:semantic_embeddable_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	seed := []struct {
		title      string
		archivedAt any
		trashedAt  any
		encrypted  int
	}{
		{"ativo", nil, nil, 0},
		{"arquivado", "2026-01-01T00:00:00Z", nil, 0},
		{"lixeira", nil, "2026-01-01T00:00:00Z", 0},
		{"protegido", nil, nil, 1},
	}
	for _, s := range seed {
		if _, err := db.Exec(`
			INSERT INTO documents (title, archived_at, trashed_at, encrypted, created_at, updated_at)
			VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
			s.title, s.archivedAt, s.trashedAt, s.encrypted); err != nil {
			t.Fatalf("seed %s: %v", s.title, err)
		}
	}

	rows, err := db.Query(`SELECT title FROM documents WHERE ` + embeddableWhere + ` ORDER BY title`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	want := []string{"arquivado", "ativo"}
	if len(got) != len(want) {
		t.Fatalf("embeddable docs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("embeddable docs: got %v, want %v", got, want)
		}
	}
}
