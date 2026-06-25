package integration_test

import (
	"net/http"
	"testing"
)

func TestSemanticFilterUnavailable(t *testing.T) {
	client := loginClient(t)
	resp, err := client.Get(ts.URL + "/api/tree?q=foo&mode=semantic")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("semantic sem GEMINI_API_KEY: want 503, got %d", resp.StatusCode)
	}
}

func TestLexicalFilterStillWorks(t *testing.T) {
	client := loginClient(t)
	resp, err := client.Get(ts.URL + "/api/tree?q=foo") // mode ausente -> léxico
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("léxico: want 200, got %d", resp.StatusCode)
	}
}
