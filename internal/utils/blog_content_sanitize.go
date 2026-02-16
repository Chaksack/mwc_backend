package utils

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	youtubeEmbedSrcRe = regexp.MustCompile(`^https://(www\.)?youtube\.com/embed/[a-zA-Z0-9_-]+(\?.*)?$`)
)

// SanitizeBlogContent sanitizes HTML coming from a WYSIWYG/word editor.
// It allows common formatting tags, inline images (<img src="https://...") and
// YouTube embeds via iframe src="https://www.youtube.com/embed/<id>".
func SanitizeBlogContent(rawHTML string) string {
	rawHTML = strings.TrimSpace(rawHTML)
	if rawHTML == "" {
		return ""
	}

	policy := bluemonday.UGCPolicy()
	policy.RequireParseableURLs(true)
	policy.AllowURLSchemes("http", "https")

	// Headings & structure
	policy.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "br", "hr",
		"div", "span",
		"blockquote",
		"ul", "ol", "li",
		"pre", "code",
		"table", "thead", "tbody", "tr", "th", "td",
		"figure", "figcaption",
	)

	// Text formatting
	policy.AllowElements("strong", "b", "em", "i", "u", "s", "mark")

	// Links
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowAttrs("title").OnElements("a")
	policy.AllowAttrs("target").Matching(regexp.MustCompile(`^(_blank|_self)$`)).OnElements("a")
	policy.AllowAttrs("rel").OnElements("a")

	// Images
	policy.AllowElements("img")
	policy.AllowAttrs("src").OnElements("img")
	policy.AllowAttrs("alt", "title").OnElements("img")
	policy.AllowAttrs("width", "height").Matching(regexp.MustCompile(`^[0-9]{1,4}$`)).OnElements("img")
	policy.AllowAttrs("loading").Matching(regexp.MustCompile(`^(lazy|eager)$`)).OnElements("img")

	// YouTube embed iframe (tight allowlist)
	policy.AllowElements("iframe")
	policy.AllowAttrs("src").Matching(youtubeEmbedSrcRe).OnElements("iframe")
	policy.AllowAttrs("width", "height").Matching(regexp.MustCompile(`^[0-9]{1,4}$`)).OnElements("iframe")
	policy.AllowAttrs("title").OnElements("iframe")
	policy.AllowAttrs("frameborder").Matching(regexp.MustCompile(`^[0-9]{1,2}$`)).OnElements("iframe")
	policy.AllowAttrs("allow").OnElements("iframe")
	policy.AllowAttrs("allowfullscreen").OnElements("iframe")
	policy.AllowAttrs("referrerpolicy").OnElements("iframe")

	return policy.Sanitize(rawHTML)
}
