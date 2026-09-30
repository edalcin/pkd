package security

import (
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// bodyURL is the rule for a Link no corpo (docs/adr/glossary.md): only http://
// and https://. The editor's Link.shouldAutoLink uses the same rule
// (frontend/src/lib/editor/extensions.js); change both together.
var bodyURL = regexp.MustCompile(`(?i)https?://[^\s\p{Z}<>"']+`)

// LinkifyHTML turns each http(s) URL in the text of s into
// <a href target rel>, the same markup the editor's autolink writes. Text in
// <a>, <code> and <pre> stays as is, so a second pass changes nothing.
func LinkifyHTML(s string) string {
	if !strings.Contains(strings.ToLower(s), "http") {
		return s
	}
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				return s
			}
			return b.String()
		case html.StartTagToken, html.EndTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "a", "code", "pre":
				if tt == html.StartTagToken {
					skip++
				} else if skip > 0 {
					skip--
				}
			}
		case html.TextToken:
			if skip == 0 {
				if text := string(z.Text()); bodyURL.MatchString(text) {
					b.WriteString(linkifyText(text))
					continue
				}
			}
		}
		b.Write(z.Raw())
	}
}

// linkifyText links the URLs of unescaped text and returns escaped HTML.
func linkifyText(text string) string {
	var b strings.Builder
	last := 0
	for _, m := range bodyURL.FindAllStringIndex(text, -1) {
		u := trimURL(text[m[0]:m[1]])
		b.WriteString(html.EscapeString(text[last:m[0]]))
		e := html.EscapeString(u)
		b.WriteString(`<a href="` + e + `" target="_blank" rel="noopener noreferrer">` + e + `</a>`)
		last = m[0] + len(u)
	}
	b.WriteString(html.EscapeString(text[last:]))
	return b.String()
}

// trimURL drops sentence punctuation after a URL ("veja https://x.com.").
// A ")" stays when it closes a "(" of the URL (Wikipedia-style paths).
func trimURL(u string) string {
	for len(u) > 0 {
		c := u[len(u)-1]
		if strings.IndexByte(".,;:!?]}", c) >= 0 ||
			(c == ')' && strings.Count(u, "(") < strings.Count(u, ")")) {
			u = u[:len(u)-1]
			continue
		}
		return u
	}
	return u
}
