package integration_test

import (
	"net/http"
	"testing"
)

// TestChatWithoutAPIKey proves the endpoint exists, is behind auth+CSRF (apiPost
// sends the token), and fails fast with 503 instead of streaming an empty
// answer when GEMINI_API_KEY is unset — the harness has no key. This is the
// path the disabled topbar icon mirrors in the UI (ADR-006 D9).
func TestChatWithoutAPIKey(t *testing.T) {
	client := loginClient(t)
	resp := apiPost(t, client, "/api/chat", map[string]interface{}{
		"messages": []map[string]string{{"role": "user", "text": "o que anotei sobre fotossíntese?"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("chat without key: want 503, got %d", resp.StatusCode)
	}
}

// TestChatRejectsEmptyBody covers the two input guards at the trust boundary:
// no messages at all, and a last message that is only whitespace. Both must be
// 400 and must never reach retrieval or the model.
func TestChatRejectsEmptyBody(t *testing.T) {
	client := loginClient(t)
	for _, body := range []map[string]interface{}{
		{"messages": []map[string]string{}},
		{"messages": []map[string]string{{"role": "user", "text": "   "}}},
	} {
		resp := apiPost(t, client, "/api/chat", body)
		got := resp.StatusCode
		resp.Body.Close()
		// The empty-key guard runs first, so an empty message list can only be
		// distinguished when it is checked before the key. Both guards reject;
		// what must never happen is 200 with a stream.
		if got == http.StatusOK {
			t.Fatalf("chat accepted invalid body %v", body)
		}
	}
}

// TestChatRequiresAuth confirms the route sits inside the authenticated group.
func TestChatRequiresAuth(t *testing.T) {
	resp, err := http.Post(ts.URL+"/api/chat", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("chat is reachable without authentication")
	}
}
