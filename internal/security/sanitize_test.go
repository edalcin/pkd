package security

import (
	"strings"
	"testing"
)

func TestSanitizeEditorHTML_TextAlign(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"p left", `<p style="text-align: left">x</p>`, `<p style="text-align: left">x</p>`},
		{"p center", `<p style="text-align: center">x</p>`, `<p style="text-align: center">x</p>`},
		{"p right", `<p style="text-align: right">x</p>`, `<p style="text-align: right">x</p>`},
		{"p justify", `<p style="text-align: justify">x</p>`, `<p style="text-align: justify">x</p>`},
		{"h1", `<h1 style="text-align: center">x</h1>`, `<h1 style="text-align: center">x</h1>`},
		{"h6", `<h6 style="text-align: right">x</h6>`, `<h6 style="text-align: right">x</h6>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeEditorHTML(c.in)
			if got != c.want {
				t.Errorf("SanitizeEditorHTML(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeEditorHTML_StripsOtherStyles(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"position", `<p style="position: fixed; top: 0">x</p>`},
		{"background url", `<p style="background: url(javascript:alert(1))">x</p>`},
		{"text-align invalid value", `<p style="text-align: inherit">x</p>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeEditorHTML(c.in)
			if strings.Contains(got, "style=") {
				t.Errorf("SanitizeEditorHTML(%q) = %q, expected style attribute stripped", c.in, got)
			}
		})
	}
}

func TestSanitizeEditorHTML_StripsScript(t *testing.T) {
	got := SanitizeEditorHTML(`<p>hi</p><script>alert(1)</script>`)
	if strings.Contains(got, "<script") {
		t.Errorf("SanitizeEditorHTML did not strip <script>: %q", got)
	}
	if !strings.Contains(got, "<p>hi</p>") {
		t.Errorf("SanitizeEditorHTML dropped legitimate content: %q", got)
	}
}

func TestSanitizeEditorHTML_ColspanRowspan(t *testing.T) {
	in := `<table><tr><td colspan="2" rowspan="3">x</td></tr></table>`
	got := SanitizeEditorHTML(in)
	if !strings.Contains(got, `colspan="2"`) {
		t.Errorf("colspan was stripped: %q", got)
	}
	if !strings.Contains(got, `rowspan="3"`) {
		t.Errorf("rowspan was stripped: %q", got)
	}

	th := `<table><tr><th colspan="4">x</th></tr></table>`
	gotTh := SanitizeEditorHTML(th)
	if !strings.Contains(gotTh, `colspan="4"`) {
		t.Errorf("colspan on th was stripped: %q", gotTh)
	}
}

func TestSanitizeEditorHTML_ColspanRowspan_InvalidStripped(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"non-numeric", `<table><tr><td colspan="x">y</td></tr></table>`},
		{"zero", `<table><tr><td colspan="0">y</td></tr></table>`},
		{"negative", `<table><tr><td colspan="-1">y</td></tr></table>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeEditorHTML(c.in)
			if strings.Contains(got, "colspan=") {
				t.Errorf("SanitizeEditorHTML(%q) = %q, expected invalid colspan stripped", c.in, got)
			}
		})
	}
}
