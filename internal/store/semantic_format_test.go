package store

import "testing"

// TestEmbedTextFormat covers ADR-004 D1/D2: the asymmetric query/document pair
// required by gemini-embedding-2, which is now the only model (see
// EmbedModelName). Query and document formats must stay paired or similarity
// degrades silently.
//
// White-box: the helpers are unexported, so this is the only reachable seam.
func TestEmbedTextFormat(t *testing.T) {
	cases := []struct{ got, want string }{
		{embedDocText("Fotossíntese", "corpo"), "title: Fotossíntese | text: corpo"},
		{embedDocText("", "corpo"), "title: none | text: corpo"},
		{embedQueryText("como planta respira"), "task: search result | query: como planta respira"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q want %q", i, c.got, c.want)
		}
	}
}
