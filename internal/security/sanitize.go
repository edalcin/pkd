package security

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	editorPolicy     *bluemonday.Policy
	publicSharePolicy *bluemonday.Policy
)

func init() {
	// EditorPolicy: generous set for the rich editor — preserves formatting,
	// images with resize widths, tables, code blocks, and links.
	editorPolicy = bluemonday.NewPolicy()
	editorPolicy.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "br", "hr",
		"strong", "em", "u", "s", "sub", "sup",
		"ul", "ol", "li",
		"blockquote",
		"pre", "code",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td",
		"figure", "figcaption",
		"div", "span",
	)
	editorPolicy.AllowAttrs("href", "rel", "target").OnElements("a")
	editorPolicy.AllowURLSchemes("http", "https", "mailto")
	editorPolicy.AllowAttrs("src", "alt", "width", "height").OnElements("img")
	// Allow width/height style properties on img/figure (CKEditor sets "width:Xpx" for resize)
	editorPolicy.AllowStyles("width", "height").
		Matching(regexp.MustCompile(`^\d+(%|px|em|rem|vw|vh)?$`)).
		OnElements("img", "figure")
	editorPolicy.AllowAttrs("class").OnElements(
		"code", "pre", "span", "div", "table", "th", "td",
	)
	// Data attributes used by CKEditor
	editorPolicy.AllowDataAttributes()

	// PublicSharePolicy: tighter — no event handlers, no JS hrefs,
	// no style attributes (prevents CSS injection), no data attributes.
	publicSharePolicy = bluemonday.NewPolicy()
	publicSharePolicy.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "br", "hr",
		"strong", "em", "u", "s", "sub", "sup",
		"ul", "ol", "li",
		"blockquote",
		"pre", "code",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td",
		"figure", "figcaption",
	)
	publicSharePolicy.AllowAttrs("href", "rel").OnElements("a")
	publicSharePolicy.AllowURLSchemes("http", "https", "mailto")
	publicSharePolicy.AllowAttrs("src", "alt", "width", "height").OnElements("img")
}

// SanitizeEditorHTML sanitizes HTML from the editor before storing it.
// It preserves rich formatting while stripping XSS vectors.
func SanitizeEditorHTML(html string) string {
	return editorPolicy.Sanitize(html)
}

// SanitizePublicHTML sanitizes HTML for the public share view.
// It applies a stricter policy — no styles, no data attributes.
func SanitizePublicHTML(html string) string {
	return publicSharePolicy.Sanitize(html)
}

// ExtractPlainText strips all HTML tags from the input and returns plain text.
// Used to populate body_text for FTS5 indexing and search snippets.
func ExtractPlainText(html string) string {
	stripped := bluemonday.StrictPolicy().Sanitize(html)
	// Collapse runs of whitespace
	parts := strings.Fields(stripped)
	return strings.Join(parts, " ")
}
