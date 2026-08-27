package store

import "testing"

// TestEmbedTextFormat covers ADR-004 D1/D2: the asymmetric query/document pair
// for gemini-embedding-2, and the guarantee that every other model keeps the
// pre-ADR-004 plain format — which is what makes selecting gemini-embedding-001
// a real rollback rather than a third, untested behaviour.
//
// White-box: the helpers are unexported, so this is the only reachable seam.
func TestEmbedTextFormat(t *testing.T) {
	const t2, t1 = "models/gemini-embedding-2", "models/gemini-embedding-001"
	cases := []struct{ got, want string }{
		{embedDocText(t2, "Fotossíntese", "corpo"), "title: Fotossíntese | text: corpo"},
		{embedDocText(t2, "", "corpo"), "title: none | text: corpo"},
		{embedQueryText(t2, "como planta respira"), "task: search result | query: como planta respira"},
		// 001 must stay byte-identical to pre-ADR-004 behaviour: the rollback guarantee.
		{embedDocText(t1, "Fotossíntese", "corpo"), "Fotossíntese\ncorpo"},
		{embedQueryText(t1, "como planta respira"), "como planta respira"},
		{embedDocText("models/embedding-001", "T", "b"), "T\nb"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q want %q", i, c.got, c.want)
		}
	}
}
