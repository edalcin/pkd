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
		"mark",
	)
	editorPolicy.AllowAttrs("href", "rel", "target").OnElements("a")
	editorPolicy.AllowURLSchemes("http", "https", "mailto")
	editorPolicy.AllowRelativeURLs(true)
	editorPolicy.AllowAttrs("src", "alt", "width", "height").OnElements("img")
	// Allow width/height style properties on img/figure (CKEditor sets "width:Xpx" for resize)
	editorPolicy.AllowStyles("width", "height").
		Matching(regexp.MustCompile(`^\d+(%|px|em|rem|vw|vh)?$`)).
		OnElements("img", "figure")
	// Allow background-color on mark (TipTap highlight extension uses inline style)
	editorPolicy.AllowStyles("background-color", "color").
		Matching(regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$|^rgb\(\d+,\s*\d+,\s*\d+\)$|^rgba\(\d+,\s*\d+,\s*\d+,\s*[\d.]+\)$|^inherit$|^transparent$`)).
		OnElements("mark")
	editorPolicy.AllowAttrs("class").OnElements(
		"code", "pre", "span", "div", "table", "th", "td",
	)
	// Allow the TipTap TextAlign extension's inline style on block text elements.
	editorPolicy.AllowStyles("text-align").
		Matching(regexp.MustCompile(`^(left|center|right|justify)$`)).
		OnElements("p", "h1", "h2", "h3", "h4", "h5", "h6")
	// Allow positive-integer colspan/rowspan from the TipTap Table extension's merged cells.
	editorPolicy.AllowAttrs("colspan", "rowspan").
		Matching(regexp.MustCompile(`^[1-9][0-9]*$`)).
		OnElements("td", "th")
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
		"mark",
	)
	publicSharePolicy.AllowStyles("background-color", "color").
		Matching(regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$|^rgb\(\d+,\s*\d+,\s*\d+\)$|^rgba\(\d+,\s*\d+,\s*\d+,\s*[\d.]+\)$|^inherit$|^transparent$`)).
		OnElements("mark")
	publicSharePolicy.AllowAttrs("href", "rel").OnElements("a")
	publicSharePolicy.AllowURLSchemes("http", "https", "mailto")
	publicSharePolicy.AllowRelativeURLs(true)
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
