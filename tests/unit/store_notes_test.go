package unit_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/edalcin/pkd/internal/store"
)

func openNoteDB(t *testing.T, name string) (*store.DocumentStore, *store.TagStore) {
	t.Helper()
	db, err := store.Open("file:" + name + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewDocumentStore(db), store.NewTagStore(db)
}

func TestCreateNote_IdempotencyKey(t *testing.T) {
	docs, _ := openNoteDB(t, "note_idem")
	a, created, err := docs.CreateNote("Endereço do dentista", "hermes-abc", false, nil, nil)
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	b, created, err := docs.CreateNote("Endereço do dentista", "hermes-abc", false, nil, nil)
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("retry must return the same Nota: created=%v id=%d want %d err=%v", created, b.ID, a.ID, err)
	}
}

// createdAt/updatedAt, when supplied, override the server clock (Q11: only
// honored for Bearer requests — that gate is enforced by the handler).
func TestCreateNote_ExplicitDates(t *testing.T) {
	docs, _ := openNoteDB(t, "note_dates")
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)
	doc, created2, err := docs.CreateNote("Nota migrada", "", false, &created, &updated)
	if err != nil || !created2 {
		t.Fatalf("create: created=%v err=%v", created2, err)
	}
	if !doc.CreatedAt.Equal(created) {
		t.Errorf("created_at = %v, want %v", doc.CreatedAt, created)
	}
	if !doc.UpdatedAt.Equal(updated) {
		t.Errorf("updated_at = %v, want %v", doc.UpdatedAt, updated)
	}
	if doc.Icon != "bx-sticky-note" {
		t.Errorf("icon = %q, want bx-sticky-note (Q16)", doc.Icon)
	}
}

// Notas block order (Q4): favorites first, then created_at desc. Tag filter
// (Q5) and the archived/trashed exclusion mirror the normal tree filters.
func TestListNotes_OrderTagFilterExclusions(t *testing.T) {
	docs, tags := openNoteDB(t, "note_list")
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(title string, offsetHours int, fav bool) int64 {
		ts := t0.Add(time.Duration(offsetHours) * time.Hour)
		d, _, err := docs.CreateNote(title, "", fav, &ts, &ts)
		if err != nil {
			t.Fatal(err)
		}
		return d.ID
	}

	mk("Older", 0, false)
	mk("Newer", 2, false)
	favID := mk("Favorita mais antiga", -10, true) // favorite wins even though oldest
	archivedID := mk("Arquivada", 5, false)
	trashedID := mk("Na lixeira", 6, false)

	if _, err := docs.Archive(archivedID); err != nil {
		t.Fatal(err)
	}
	if err := docs.SoftDelete(trashedID); err != nil {
		t.Fatal(err)
	}

	list, err := docs.ListNotes(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range list {
		got = append(got, n.Title)
	}
	want := []string{"Favorita mais antiga", "Newer", "Older"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order\n got: %v\nwant: %v", got, want)
	}
	if n := favID; list[0].ID != n || !list[0].IsFavorite {
		t.Errorf("first item must be the favorite: %+v", list[0])
	}

	// Tag filter: only a tagged Nota comes back.
	taggedID := mk("Com tag proj", 1, false)
	if err := tags.SetDocumentTags(taggedID, []string{"proj", "casa"}); err != nil {
		t.Fatal(err)
	}
	tagged, err := docs.ListNotes([]string{"proj"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 1 || tagged[0].ID != taggedID {
		t.Errorf("tag filter: got %+v, want only the tagged Nota", tagged)
	} else if got := strings.Join(tagged[0].Tags, ","); got != "casa,proj" && got != "proj,casa" {
		t.Errorf("list tags: got %q, want casa+proj", got)
	}

	// Favorites-only filter.
	favOnly, err := docs.ListNotes(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(favOnly) != 1 || favOnly[0].ID != favID {
		t.Errorf("favorite filter: got %+v, want only the favorite Nota", favOnly)
	}
}

// A Nota never enters the normal tree and never gets a parent or children.
func TestNote_NoHierarchy(t *testing.T) {
	docs, _ := openNoteDB(t, "note_hier")
	note, _, err := docs.CreateNote("Nota", "", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := docs.Create(nil, "Documento")
	if err != nil {
		t.Fatal(err)
	}
	if err := docs.Move(note.ID, &doc.ID); !errors.Is(err, store.ErrNoteHierarchy) {
		t.Errorf("move note under doc: got %v", err)
	}
	if err := docs.Reorder(doc.ID, &note.ID, nil); !errors.Is(err, store.ErrNoteHierarchy) {
		t.Errorf("reorder doc under note: got %v", err)
	}
	if _, err := docs.Create(&note.ID, "Filho"); !errors.Is(err, store.ErrNoteHierarchy) {
		t.Errorf("create child of note: got %v", err)
	}
	for _, view := range []string{"active", "all"} {
		list, err := docs.ListTree(view, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range list {
			if d.ID == note.ID {
				t.Errorf("view %q: note listed in the normal tree", view)
			}
		}
	}
}

// Q3/Q18: converting a Nota to a Documento clears is_note, restores the
// Documento default icon, places it under the given parent, and it leaves
// the Notas list for the normal tree.
func TestConvertNoteToDocument(t *testing.T) {
	docs, _ := openNoteDB(t, "note_convert_doc")
	parent, err := docs.Create(nil, "Pai")
	if err != nil {
		t.Fatal(err)
	}
	note, _, err := docs.CreateNote("Nota A", "", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	converted, err := docs.ConvertNoteToDocument(note.ID, &parent.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if converted.IsNote {
		t.Error("still marked is_note after conversion")
	}
	if converted.ParentID == nil || *converted.ParentID != parent.ID {
		t.Errorf("not placed under parent: %+v", converted.ParentID)
	}
	if converted.Icon != "bx-dock-top" {
		t.Errorf("icon not reset to the Documento default: %q", converted.Icon)
	}

	list, err := docs.ListNotes(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range list {
		if n.ID == note.ID {
			t.Error("converted note still in the Notas list")
		}
	}

	tree, err := docs.ListTree("active", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range tree {
		if d.ID == note.ID {
			found = true
		}
	}
	if !found {
		t.Error("converted note not present in the normal tree")
	}
}

// Q3/Q17: converting a Nota to a Memória clears is_note, emits a Memória ID
// and sets the Data da Memória; year-only precision is enough.
func TestConvertNoteToMemory(t *testing.T) {
	docs, _ := openNoteDB(t, "note_convert_mem")
	note, _, err := docs.CreateNote("Nota B", "", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := docs.ConvertNoteToMemory(note.ID, store.MemoryDate{Year: 2020})
	if err != nil {
		t.Fatal(err)
	}
	if converted.IsNote {
		t.Error("still marked is_note after conversion")
	}
	if !strings.HasPrefix(converted.MemoryID, "MEM-2020-") || len(converted.MemoryID) != len("MEM-2020-")+6 {
		t.Errorf("unexpected memory id: %q", converted.MemoryID)
	}
	if converted.Icon != "bx-calendar-event" {
		t.Errorf("icon not set to the Memória default: %q", converted.Icon)
	}
	if converted.AssocYear == nil || *converted.AssocYear != 2020 {
		t.Errorf("assoc_year not set: %+v", converted.AssocYear)
	}

	list, err := docs.ListMemories()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.ID == note.ID {
			found = true
		}
	}
	if !found {
		t.Error("converted note not present in the MC list")
	}
}
