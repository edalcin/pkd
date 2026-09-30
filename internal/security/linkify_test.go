package security

import "testing"

func TestSanitizeEditorHTMLLinksBodyURLs(t *testing.T) {
	const a = `<a href="https://x.com/a?b=1&amp;c=2" target="_blank" rel="noopener noreferrer">https://x.com/a?b=1&amp;c=2</a>`
	cases := []struct{ name, in, want string }{
		{"plain text from capture", `veja https://x.com/a?b=1&c=2.`, `veja ` + a + `.`},
		{"inside paragraph", `<p>Ver https://x.com/a?b=1&amp;c=2, ok</p>`, `<p>Ver ` + a + `, ok</p>`},
		{"existing link unchanged", `<p>` + a + `</p>`, `<p>` + a + `</p>`},
		{"code unchanged", `<pre><code>curl https://x.com</code></pre>`, `<pre><code>curl https://x.com</code></pre>`},
		{"no scheme is not a link", `<p>www.x.com e x.com</p>`, `<p>www.x.com e x.com</p>`},
		{"balanced paren kept", `<p>(https://w.org/F_(b))</p>`,
			`<p>(<a href="https://w.org/F_(b)" target="_blank" rel="noopener noreferrer">https://w.org/F_(b)</a>)</p>`},
		{"stops at nbsp", "<p>https://x.com&nbsp;fim</p>",
			"<p><a href=\"https://x.com\" target=\"_blank\" rel=\"noopener noreferrer\">https://x.com</a>\u00a0fim</p>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeEditorHTML(c.in)
			if got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
			if again := SanitizeEditorHTML(got); again != got {
				t.Errorf("second pass changed output: %q", again)
			}
		})
	}
}
