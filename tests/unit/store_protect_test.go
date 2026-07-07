package unit_test

import (
	"errors"
	"testing"

	"github.com/edalcin/pkd/internal/store"
)

// openProtectTestDB opens a fresh in-memory SQLite DB dedicated to this file's
// tests (unique DSN so it never shares state with other tests/unit files that
// also use store.Open's in-memory URI).
func openProtectTestDB(t *testing.T) *store.DocumentStore {
	t.Helper()
	db, err := store.Open("file:store_protect_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewDocumentStore(db)
}

// TestProtectUnprotect_RoundTripAndFTSVisibility exercises the full
// Protect/Unprotect lifecycle at the store layer: encrypting a document swaps
// its body for ciphertext and drops it from full-text search; unprotecting
// restores the plaintext body and re-indexes it.
func TestProtectUnprotect_RoundTripAndFTSVisibility(t *testing.T) {
	db, err := store.Open("file:store_protect_fts_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	docs := store.NewDocumentStore(db)
	search := store.NewSearchStore(db)

	doc, err := docs.Create(nil, "Segredo Test Doc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const plainHTML = "<p>Historia sobre o unicorniomagico da floresta.</p>"
	const plainText = "Historia sobre o unicorniomagico da floresta."

	updated, err := docs.Update(doc.ID, doc.Version, doc.Title, plainHTML, plainText, doc.Icon)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Mirror what the real PUT handler does after a successful Update: index
	// the new body into FTS (internal/server/handlers_documents.go calls
	// s.search.IndexDoc(doc.ID, doc.Title, plainText, nil) right after
	// UpdateAndSync — Update itself does not touch the FTS index).
	if err := search.IndexDoc(updated.ID, updated.Title, plainText, nil); err != nil {
		t.Fatalf("IndexDoc: %v", err)
	}

	// Baseline: searchable before protecting.
	hits, err := search.Search("unicorniomagico", 10)
	if err != nil {
		t.Fatalf("Search (baseline): %v", err)
	}
	if !containsHit(hits, doc.ID) {
		t.Fatalf("expected doc %d in baseline search results, got %+v", doc.ID, hits)
	}

	// Protect: encrypts at rest and removes from FTS.
	const cipherBlob = "fake-cipher-blob-not-real-aes"
	if _, err := docs.Protect(doc.ID, cipherBlob); err != nil {
		t.Fatalf("Protect: %v", err)
	}

	protected, err := docs.GetByID(doc.ID)
	if err != nil {
		t.Fatalf("GetByID after Protect: %v", err)
	}
	if !protected.Encrypted {
		t.Error("expected Encrypted=true after Protect")
	}
	if protected.BodyHTML != cipherBlob {
		t.Errorf("expected BodyHTML=%q after Protect, got %q", cipherBlob, protected.BodyHTML)
	}
	if protected.BodyText != "" {
		t.Errorf("expected BodyText='' after Protect, got %q", protected.BodyText)
	}

	hits, err = search.Search("unicorniomagico", 10)
	if err != nil {
		t.Fatalf("Search (after Protect): %v", err)
	}
	if containsHit(hits, doc.ID) {
		t.Fatalf("expected doc %d to be removed from search results after Protect, got %+v", doc.ID, hits)
	}

	// Unprotect: restores plaintext and re-indexes.
	if _, err := docs.Unprotect(doc.ID, plainHTML, plainText); err != nil {
		t.Fatalf("Unprotect: %v", err)
	}

	unprotected, err := docs.GetByID(doc.ID)
	if err != nil {
		t.Fatalf("GetByID after Unprotect: %v", err)
	}
	if unprotected.Encrypted {
		t.Error("expected Encrypted=false after Unprotect")
	}
	if unprotected.BodyHTML != plainHTML {
		t.Errorf("expected BodyHTML=%q after Unprotect, got %q", plainHTML, unprotected.BodyHTML)
	}

	hits, err = search.Search("unicorniomagico", 10)
	if err != nil {
		t.Fatalf("Search (after Unprotect): %v", err)
	}
	if !containsHit(hits, doc.ID) {
		t.Fatalf("expected doc %d back in search results after Unprotect, got %+v", doc.ID, hits)
	}
}

// TestProtect_NonExistentDocument confirms Protect surfaces store.ErrNotFound
// for a document ID that was never created.
func TestProtect_NonExistentDocument(t *testing.T) {
	docs := openProtectTestDB(t)

	_, err := docs.Protect(999999, "irrelevant-cipher-blob")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func containsHit(hits []store.SearchHit, id int64) bool {
	for _, h := range hits {
		if h.ID == id {
			return true
		}
	}
	return false
}
