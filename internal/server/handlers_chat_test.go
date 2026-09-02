package server

import (
	"errors"
	"strings"
	"testing"
)

// TestRetrievalQuery proves the multi-turn retrieval query (ADR-006 D6) carries
// the subject from earlier turns: "e sobre a fase escura?" alone retrieves
// badly, so the trailing user messages are concatenated. Model turns are
// excluded — a long generated answer would swamp the query — and only the last
// chatHistoryTurns user messages participate.
func TestRetrievalQuery(t *testing.T) {
	msgs := []chatMessage{
		{Role: "user", Text: "o que anotei sobre fotossíntese?"},
		{Role: "model", Text: "A fotossíntese ocorre em duas fases..."},
		{Role: "user", Text: "e sobre a fase escura?"},
	}
	q := retrievalQuery(msgs)
	if !strings.Contains(q, "fotossíntese") {
		t.Errorf("follow-up query lost the earlier subject: %q", q)
	}
	if !strings.Contains(q, "fase escura") {
		t.Errorf("query missing the current question: %q", q)
	}
	if strings.Contains(q, "duas fases") {
		t.Errorf("model turn must not enter the query: %q", q)
	}

	// Only the last chatHistoryTurns user messages participate.
	many := []chatMessage{
		{Role: "user", Text: "antiga"},
		{Role: "user", Text: "t1"},
		{Role: "user", Text: "t2"},
		{Role: "user", Text: "t3"},
	}
	if q := retrievalQuery(many); strings.Contains(q, "antiga") {
		t.Errorf("query kept more than %d user turns: %q", chatHistoryTurns, q)
	}

	// Blank turns are dropped, never emitted as empty tokens.
	if q := retrievalQuery([]chatMessage{{Role: "user", Text: "  "}, {Role: "user", Text: "x"}}); q != "x" {
		t.Errorf("blank turn leaked into query: %q", q)
	}
}

// TestChatErrorMessage locks the ordering that a smoke test exposed: Gemini
// reports an invalid API key as 400 INVALID_ARGUMENT, so the key case MUST be
// checked before the generic 400 — otherwise the user is told the prompt was
// too long and goes chasing the wrong problem.
func TestChatErrorMessage(t *testing.T) {
	cases := []struct {
		err  string
		want string
	}{
		{`gemini chat: status 400: {"error":{"status":"INVALID_ARGUMENT","details":[{"reason":"API_KEY_INVALID"}]}}`, "GEMINI_API_KEY"},
		{"gemini chat: status 429: quota", "Limite de uso"},
		{"gemini chat: status 400: prompt is too long", "excedeu o limite"},
		{"gemini chat: status 404: model not found", "não está disponível"},
		{"dial tcp: connection refused", "Falha ao gerar"},
	}
	for _, c := range cases {
		got := chatErrorMessage(errors.New(c.err))
		if !strings.Contains(got, c.want) {
			t.Errorf("chatErrorMessage(%q)\n got: %q\nwant substring: %q", c.err, got, c.want)
		}
	}
}
