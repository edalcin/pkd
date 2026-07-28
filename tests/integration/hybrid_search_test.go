package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestHybridSearchDegradesToLexical proves three things at once: no more 503
// without GEMINI_API_KEY; the lexical retriever is now FTS5 (the old LIKE
// would not match "fotossintese" against "Fotossíntese" — documents_fts uses
// tokenize='unicode61 remove_diacritics 2'); and RRF degrades to the lexical
// order when the semantic leg is empty (no API key in this test harness).
func TestHybridSearchDegradesToLexical(t *testing.T) {
	client := loginClient(t)
	created := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Fotossíntese das plantas",
	}))
	id := int64(created["id"].(float64))

	resp, err := client.Get(ts.URL + "/api/tree?q=fotossintese&view=all")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hybrid search: want 200, got %d", resp.StatusCode)
	}
	var nodes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatal("decode:", err)
	}
	found := false
	for _, n := range nodes {
		if int64(n["id"].(float64)) == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected doc %d in results %v", id, nodes)
	}
}

// TestHybridSearchNoMatch confirms an empty result is serialized as [] never null.
func TestHybridSearchNoMatch(t *testing.T) {
	client := loginClient(t)
	resp, err := client.Get(ts.URL + "/api/tree?q=zzzznaoexistezzzz&view=all")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("no-match search: want 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(body))
	if got != "[]" {
		t.Fatalf("no-match search: want body `[]`, got %q", got)
	}
}

// TestHybridSearchIgnoresLegacyModeParam confirms a stale ?mode=semantic from
// an old client is simply ignored (always hybrid now), never a 503.
func TestHybridSearchIgnoresLegacyModeParam(t *testing.T) {
	client := loginClient(t)
	created := decodeDoc(t, apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Fotossíntese e clorofila",
	}))
	id := int64(created["id"].(float64))

	resp, err := client.Get(ts.URL + "/api/tree?q=fotossintese&view=all&mode=semantic")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("legacy mode param: want 200, got %d", resp.StatusCode)
	}
	var nodes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatal("decode:", err)
	}
	found := false
	for _, n := range nodes {
		if int64(n["id"].(float64)) == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected doc %d in results %v", id, nodes)
	}
}
