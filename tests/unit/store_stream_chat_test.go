package unit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edalcin/pkd/internal/store"
)

// TestStreamChat_ParsesSSE exercises the SSE frame parser against the shapes
// the Gemini API actually emits: a normal text frame, a frame with no text
// part, a malformed frame (must be skipped, never abort a good answer), and a
// last frame carrying finishReason. It also asserts the two contract details
// that are easy to get wrong: authentication travels in the x-goog-api-key
// header (never ?key=, ADR-006 D10) and the URL asks for alt=sse.
func TestStreamChat_ParsesSSE(t *testing.T) {
	var gotAuth, gotQuery, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-goog-api-key")
		gotQuery = r.URL.RawQuery
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"Segundo "}],"role":"model"}}]}`,
			`data: {"candidates":[{"content":{"parts":[]}}]}`,
			`data: {oops`,
			`data: {"candidates":[{"content":{"parts":[{"text":"Fotossíntese."}]},"finishReason":"STOP"}]}`,
		}
		for _, f := range frames {
			_, _ = w.Write([]byte(f + "\n\n"))
		}
	}))
	defer srv.Close()

	// Point the client at the fake server by overriding the base URL through
	// the exported test hook.
	restore := store.SetGeminiBaseURLForTest(srv.URL + "/v1beta/")
	defer restore()

	var got strings.Builder
	err := store.StreamChat(context.Background(), "secret-key", store.ChatModelFlash,
		"regras de sistema",
		[]store.ChatDoc{{ID: 1, Title: "Fotossíntese", Body: "corpo do documento"}},
		[]store.ChatMessage{{Role: "user", Text: "o que sei disso?"}},
		4096,
		func(s string) error { got.WriteString(s); return nil })
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if got.String() != "Segundo Fotossíntese." {
		t.Errorf("chunks: got %q, want %q", got.String(), "Segundo Fotossíntese.")
	}
	if gotAuth != "secret-key" {
		t.Errorf("auth header: got %q, want the api key", gotAuth)
	}
	if gotQuery != "alt=sse" {
		t.Errorf("query: got %q, want alt=sse", gotQuery)
	}
	if !strings.Contains(gotBody, "corpo do documento") {
		t.Error("grounding document body missing from the request")
	}
	if !strings.Contains(gotBody, "systemInstruction") {
		t.Error("systemInstruction missing from the request")
	}
	if strings.Contains(gotBody, "temperature") {
		t.Error("temperature must not be sent (ADR-006 D12)")
	}
}

// TestStreamChat_SafetyBlock proves a SAFETY finishReason surfaces as an error
// instead of an answer that silently stops mid-sentence.
func TestStreamChat_SafetyBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`data: {"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}` + "\n\n"))
	}))
	defer srv.Close()
	restore := store.SetGeminiBaseURLForTest(srv.URL + "/v1beta/")
	defer restore()

	err := store.StreamChat(context.Background(), "k", store.ChatModelFlash, "sys", nil,
		[]store.ChatMessage{{Role: "user", Text: "x"}}, 100, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "safety") {
		t.Fatalf("want safety error, got %v", err)
	}
}
