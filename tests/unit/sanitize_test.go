package unit_test

import (
	"strings"
	"testing"

	"github.com/edalcin/pkd/internal/security"
)

func TestSanitizeEditorHTML_StripScript(t *testing.T) {
	payloads := []string{
		`<script>alert(1)</script>`,
		`<img onerror="alert(1)" src="x">`,
		`<a href="javascript:alert(1)">click</a>`,
		`<svg><foreignObject><script>alert(1)</script></foreignObject></svg>`,
		`<div style="background:url(javascript:alert(1))">x</div>`,
		`<p onmouseover="alert(1)">hover</p>`,
	}
	for _, payload := range payloads {
		out := security.SanitizeEditorHTML(payload)
		if strings.Contains(out, "alert(1)") {
			t.Errorf("XSS not stripped in output: %q (input: %q)", out, payload)
		}
		if strings.Contains(out, "javascript:") {
			t.Errorf("javascript: URI not stripped: %q", out)
		}
	}
}

func TestSanitizeEditorHTML_PreservesFormatting(t *testing.T) {
	input := `<h1>Title</h1><p>Hello <strong>world</strong>!</p><ul><li>item</li></ul>`
	out := security.SanitizeEditorHTML(input)
	for _, want := range []string{"<h1>", "<strong>", "<ul>", "<li>"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in sanitized output, got: %q", want, out)
		}
	}
}

func TestSanitizeEditorHTML_PreservesImageWidth(t *testing.T) {
	input := `<img src="/api/attachments/1" alt="photo" style="width:400px">`
	out := security.SanitizeEditorHTML(input)
	// bluemonday normalizes "width:400px" → "width: 400px" (adds space after colon)
	if !strings.Contains(out, "width") || !strings.Contains(out, "400px") {
		t.Errorf("expected image width style preserved, got: %q", out)
	}
	if strings.Contains(out, "<script") || strings.Contains(out, "onerror") {
		t.Errorf("unexpected dangerous content in output: %q", out)
	}
}

func TestPublicSharePolicy_StripsStyle(t *testing.T) {
	input := `<p style="color:red">text</p>`
	out := security.SanitizePublicHTML(input)
	if strings.Contains(out, "style=") {
		t.Errorf("expected style stripped in public policy, got: %q", out)
	}
}

func TestExtractPlainText(t *testing.T) {
	input := `<h1>Hello</h1><p>World <em>foo</em></p>`
	out := security.ExtractPlainText(input)
	if !strings.Contains(out, "Hello") || !strings.Contains(out, "World") || !strings.Contains(out, "foo") {
		t.Errorf("ExtractPlainText: missing expected words in %q", out)
	}
	if strings.Contains(out, "<") {
		t.Errorf("ExtractPlainText: HTML tags remain: %q", out)
	}
}
