package unit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/edalcin/pkd/internal/store"
)

func ip(v int) *int { return &v }

func openMemStore(t *testing.T, name string) *store.DocumentStore {
	t.Helper()
	db, err := store.Open("file:" + name + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewDocumentStore(db)
}

func TestMemoryDate_Validate(t *testing.T) {
	cases := []struct {
		name string
		d    store.MemoryDate
		ok   bool
	}{
		{"year only", store.MemoryDate{Year: 1850}, true},
		{"leap day", store.MemoryDate{Year: 2024, Month: ip(2), Day: ip(29)}, true},
		{"31 Feb", store.MemoryDate{Year: 2026, Month: ip(2), Day: ip(31)}, false},
		{"29 Feb non-leap", store.MemoryDate{Year: 2026, Month: ip(2), Day: ip(29)}, false},
		{"day without month", store.MemoryDate{Year: 2026, Day: ip(3)}, false},
		{"hour without day", store.MemoryDate{Year: 2026, Month: ip(9), Hour: ip(14)}, false},
		{"period without day", store.MemoryDate{Year: 2026, Month: ip(9), Period: "almoco"}, false},
		{"hour and period", store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Hour: ip(12), Period: "almoco"}, false},
		{"minute without hour", store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Minute: ip(30)}, false},
		{"hour 24", store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Hour: ip(24)}, false},
		{"unknown period", store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Period: "brunch"}, false},
	}
	for _, c := range cases {
		if err := c.d.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// The ID projects the known precision, writes a Período as its start hour
// (ADR-007) and stays valid under the canonical regex.
func TestCreateMemory_IDFormat(t *testing.T) {
	s := openMemStore(t, "mem_idformat")
	cases := []struct {
		d      store.MemoryDate
		prefix string
	}{
		{store.MemoryDate{Year: 1850}, "MEM-1850-"},
		{store.MemoryDate{Year: 2026, Month: ip(9)}, "MEM-2026-09-"},
		{store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Period: "almoco"}, "MEM-2026-09-23T12-"},
		{store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Period: "lanche"}, "MEM-2026-09-23T16-"},
		{store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(24), Hour: ip(14), Minute: ip(30)}, "MEM-2026-09-24T1430-"},
	}
	for i, c := range cases {
		doc, _, err := s.CreateMemory("m"+string(rune('a'+i)), c.d, "")
		if err != nil {
			t.Fatalf("CreateMemory: %v", err)
		}
		if !strings.HasPrefix(doc.MemoryID, c.prefix) || len(doc.MemoryID) != len(c.prefix)+6 {
			t.Errorf("memory_id %q, want prefix %q + 6 chars", doc.MemoryID, c.prefix)
		}
		if got, ok := store.NormalizeMemoryID(strings.ToLower(doc.MemoryID)); !ok || got != doc.MemoryID {
			t.Errorf("NormalizeMemoryID(lower %q) = %q, %v", doc.MemoryID, got, ok)
		}
	}
	if got, ok := store.NormalizeMemoryID("mem-2026-09-7qf3lo"); !ok || got != "MEM-2026-09-7QF310" {
		t.Errorf("Crockford decode: got %q, %v", got, ok)
	}
	if _, ok := store.NormalizeMemoryID("MEM-2026-09-23TAL-7QF3K9"); ok {
		t.Error("letter period code must be rejected by the canonical regex")
	}
}

func TestCreateMemory_IdempotencyKey(t *testing.T) {
	s := openMemStore(t, "mem_idem")
	d := store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Period: "almoco"}
	a, created, err := s.CreateMemory("Almoço no Rascal", d, "hermes-123")
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	b, created, err := s.CreateMemory("Almoço no Rascal", d, "hermes-123")
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("retry must return the same Memória: created=%v id=%d want %d err=%v", created, b.ID, a.ID, err)
	}
}

// Correcting the date moves the Memória in the MC tree but never re-emits the ID.
func TestUpdateMemoryDate_KeepsID(t *testing.T) {
	s := openMemStore(t, "mem_fixdate")
	doc, _, err := s.CreateMemory("Almoço", store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(23), Period: "almoco"}, "")
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := s.UpdateMemoryDate(doc.ID, store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(22), Hour: ip(13)})
	if err != nil {
		t.Fatal(err)
	}
	if fixed.MemoryID != doc.MemoryID {
		t.Errorf("memory_id changed: %q → %q", doc.MemoryID, fixed.MemoryID)
	}
	if *fixed.AssocDay != 22 || fixed.MemoryPeriod != "" || fixed.MemoryHour == nil || *fixed.MemoryHour != 13 {
		t.Errorf("date not replaced: day=%v period=%q hour=%v", *fixed.AssocDay, fixed.MemoryPeriod, fixed.MemoryHour)
	}
}

// MC tree order: newest year/month/day first, coarse before fine, and inside
// a day: no time, then start, then longer span, then exact time.
func TestListMemories_Order(t *testing.T) {
	s := openMemStore(t, "mem_order")
	day := func(extra store.MemoryDate) store.MemoryDate {
		extra.Year, extra.Month, extra.Day = 2026, ip(9), ip(23)
		return extra
	}
	inputs := []struct {
		title string
		d     store.MemoryDate
	}{
		{"Ligação", day(store.MemoryDate{Hour: ip(12), Minute: ip(30)})},
		{"Entrega", day(store.MemoryDate{Hour: ip(12)})},
		{"Almoço", day(store.MemoryDate{Period: "almoco"})},
		{"Passeio", day(store.MemoryDate{Period: "tarde"})},
		{"Café", day(store.MemoryDate{Period: "manha"})},
		{"Consulta", day(store.MemoryDate{})},
		{"Dia 24", store.MemoryDate{Year: 2026, Month: ip(9), Day: ip(24)}},
		{"Setembro", store.MemoryDate{Year: 2026, Month: ip(9)}},
		{"Ano 2026", store.MemoryDate{Year: 2026}},
		{"Patagônia", store.MemoryDate{Year: 1998}},
	}
	for _, in := range inputs {
		if _, _, err := s.CreateMemory(in.title, in.d, ""); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListMemories()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Ano 2026", "Setembro", "Dia 24", "Consulta", "Café", "Passeio", "Almoço", "Entrega", "Ligação", "Patagônia"}
	var got []string
	for _, m := range list {
		got = append(got, m.Title)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order\n got: %v\nwant: %v", got, want)
	}
}

// A Memória never enters the normal tree and never gets a parent or children.
func TestMemory_NoHierarchy(t *testing.T) {
	s := openMemStore(t, "mem_hier")
	mem, _, err := s.CreateMemory("Almoço", store.MemoryDate{Year: 2026}, "")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.Create(nil, "Família")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Move(mem.ID, &doc.ID); !errors.Is(err, store.ErrMemoryHierarchy) {
		t.Errorf("move memory under doc: got %v", err)
	}
	if err := s.Reorder(doc.ID, &mem.ID, nil); !errors.Is(err, store.ErrMemoryHierarchy) {
		t.Errorf("reorder doc under memory: got %v", err)
	}
	if _, err := s.Create(&mem.ID, "Cardápio"); !errors.Is(err, store.ErrMemoryHierarchy) {
		t.Errorf("create child of memory: got %v", err)
	}
	for _, view := range []string{"active", "all"} {
		docs, err := s.ListTree(view, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range docs {
			if d.ID == mem.ID {
				t.Errorf("view %q: memory listed in the normal tree", view)
			}
		}
	}
}
