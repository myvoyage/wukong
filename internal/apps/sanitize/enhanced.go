// Package sanitize provides HTML sanitization functionality.
//
// enhanced.go: Advanced HTML cleaning features.
// Adds dead link removal, charset guarantee, mobile CSS injection,
// conditional comment stripping, and cleaning report statistics.
package sanitize

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ---------------------------------------------------------------------------
// Clean options and reporting.
// ---------------------------------------------------------------------------

// CleanOptions provides fine-grained control over HTML cleaning.
type CleanOptions struct {
	// KeepNoscript unwraps <noscript> content into the DOM instead of
	// removing it. Useful for JS-rendered sites that put fallback content.
	KeepNoscript bool

	// KeepMetaRefresh preserves plain <meta http-equiv="refresh">
	// (JS-based refreshes are always removed).
	KeepMetaRefresh bool

	// Banner is an HTML comment inserted at the document's top.
	// Empty string means no banner.
	Banner string

	// MobileReadable injects viewport meta and responsive CSS to make
	// old table-layout sites readable on mobile devices.
	MobileReadable bool
}

// CleanReport records statistics about what was removed during cleaning.
type CleanReport struct {
	ScriptsRemoved      int
	HandlersRemoved     int
	NoscriptRemoved     int
	NoscriptUnwrapped   int
	JSURLsNeutralized   int
	MetaRefreshRemoved  int
	DeadLinksRemoved    int
	CondCommentsRemoved int
	CharsetAdded        bool
}

// ---------------------------------------------------------------------------
// Advanced DOM-level cleaning.
// ---------------------------------------------------------------------------

// CleanHTMLWithOptions removes JavaScript and unsafe content from HTML
// using DOM tree traversal, with full control and reporting.
func CleanHTMLWithOptions(htmlStr string, opts CleanOptions) (string, CleanReport) {
	var report CleanReport

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		// Fallback: use regex-based cleaning with basic reporting.
		return cleanHTMLRegex(htmlStr, &report), report
	}

	cleanTree(doc, opts, &report)

	// Post-processing: charset, viewport, mobile CSS, banner.
	ensureCharset(doc, &report)
	if opts.MobileReadable {
		ensureViewport(doc)
		injectMobileCSS(doc)
	}
	if opts.Banner != "" {
		insertBanner(doc, opts.Banner)
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return htmlStr, report
	}
	return buf.String(), report
}

// cleanTree recursively traverses the DOM and cleans unsafe content.
// This is the primary cleaning function, handling all element types.
func cleanTree(n *html.Node, opts CleanOptions, report *CleanReport) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling

		switch c.Type {
		case html.CommentNode:
			// Remove IE conditional comments which may contain <script>.
			if isConditionalComment(c.Data) {
				n.RemoveChild(c)
				report.CondCommentsRemoved++
				c = next
				continue
			}

		case html.ElementNode:
			switch c.DataAtom {
			case atom.Script:
				n.RemoveChild(c)
				report.ScriptsRemoved++
				c = next
				continue

			case atom.Noscript:
				if opts.KeepNoscript {
					unwrapNoscript(n, c, report)
				} else {
					n.RemoveChild(c)
					report.NoscriptRemoved++
				}
				c = next
				continue

			case atom.Meta:
				if isMetaRefresh(c) {
					if !opts.KeepMetaRefresh || isJSRefresh(c) {
						n.RemoveChild(c)
						report.MetaRefreshRemoved++
						c = next
						continue
					}
				}

			case atom.Link:
				if isDeadLink(c) {
					n.RemoveChild(c)
					report.DeadLinksRemoved++
					c = next
					continue
				}

			// Unsafe elements — remove entirely for privacy/security.
			// These can embed active content, initiate network requests,
			// or alter the page context.
			case atom.Iframe, atom.Embed, atom.Object,
				atom.Applet, atom.Base:
				n.RemoveChild(c)
				c = next
				continue
			}

			// Strip event handlers from retained elements.
			report.HandlersRemoved += stripHandlers(c)

			// Neutralize javascript: URLs in retained elements.
			report.JSURLsNeutralized += neutralizeJSURLs(c)

			// Recurse into children.
			cleanTree(c, opts, report)
		}

		c = next
	}
}

// ---------------------------------------------------------------------------
// Event handler stripping.
// ---------------------------------------------------------------------------

// stripHandlers removes all on* attributes from a node.
// Returns the number of handlers removed.
func stripHandlers(n *html.Node) int {
	var keep []html.Attribute
	removed := 0
	for _, a := range n.Attr {
		if strings.HasPrefix(strings.ToLower(a.Key), "on") {
			removed++
		} else {
			keep = append(keep, a)
		}
	}
	n.Attr = keep
	return removed
}

// ---------------------------------------------------------------------------
// javascript: URL neutralization.
// ---------------------------------------------------------------------------

// jsURLAttrs maps attribute names whose values may contain javascript: URLs
// to a handling mode: "replace" means replace with "#", "remove" means
// delete the attribute entirely.
var jsURLAttrs = map[string]string{
	"href":       "replace",
	"src":        "remove",
	"action":     "remove",
	"formaction": "remove",
	"poster":     "remove",
	"data":       "remove",
	"background": "remove",
	"xlink:href": "remove",
}

// neutralizeJSURLs neutralizes javascript: URLs in element attributes.
// For href, it replaces with "#". For other attributes, it removes them.
// Returns the number of URLs neutralized.
func neutralizeJSURLs(n *html.Node) int {
	count := 0
	for i, a := range n.Attr {
		mode, ok := jsURLAttrs[strings.ToLower(a.Key)]
		if !ok {
			continue
		}
		if !isJavascriptURL(a.Val) {
			continue
		}

		count++
		if mode == "replace" {
			n.Attr[i].Val = "#"
		} else {
			// Remove the attribute by swapping with last and truncating.
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return 1 + neutralizeJSURLs(n) // Re-check remaining attrs.
		}
	}
	return count
}

// isJavascriptURL checks if a string is a javascript: pseudo-URL.
func isJavascriptURL(s string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(s))
	return strings.HasPrefix(trimmed, "javascript:")
}

// ---------------------------------------------------------------------------
// Dead link detection (optimistic resource hints that are useless offline).
// ---------------------------------------------------------------------------

// isDeadLink checks if a <link> element represents a resource hint
// that is useless in an offline mirror (preconnect, dns-prefetch,
// modulepreload, script preload).
func isDeadLink(n *html.Node) bool {
	rel := getAttrValue(n, "rel")
	if rel == "" {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(rel))
	values := strings.Fields(lower)

	for _, v := range values {
		switch v {
		case "preconnect", "dns-prefetch", "modulepreload":
			return true
		case "preload", "prefetch":
			// Only dead if loading a script.
			as := getAttrValue(n, "as")
			if strings.EqualFold(as, "script") {
				return true
			}
			href := getAttrValue(n, "href")
			if strings.HasSuffix(strings.ToLower(href), ".js") {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Conditional comment detection.
// ---------------------------------------------------------------------------

// isConditionalComment checks if a comment is an IE conditional comment.
// These may contain <script> tags that escape normal traversal.
func isConditionalComment(data string) bool {
	d := strings.TrimSpace(data)
	return strings.HasPrefix(d, "[if") ||
		strings.HasPrefix(d, "<!\\[endif\\]") ||
		strings.HasPrefix(d, "[endif]")
}

// ---------------------------------------------------------------------------
// <noscript> unwrapping.
// ---------------------------------------------------------------------------

// unwrapNoscript parses the raw text content of a <noscript> element
// as HTML and inserts the resulting nodes before the <noscript>,
// then removes the <noscript> element itself.
func unwrapNoscript(parent, noscript *html.Node, report *CleanReport) {
	// Collect text from <noscript> children.
	var text strings.Builder
	for c := noscript.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			text.WriteString(c.Data)
		}
	}

	if text.Len() == 0 {
		parent.RemoveChild(noscript)
		report.NoscriptRemoved++
		return
	}

	// Parse the text as HTML fragment (in body context).
	fragment, err := html.ParseFragment(strings.NewReader(text.String()),
		&html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		parent.RemoveChild(noscript)
		report.NoscriptRemoved++
		return
	}

	// Insert fragment nodes before <noscript>.
	for _, node := range fragment {
		parent.InsertBefore(node, noscript)
	}

	parent.RemoveChild(noscript)
	report.NoscriptUnwrapped++
}

// ---------------------------------------------------------------------------
// Meta refresh handling.
// ---------------------------------------------------------------------------

// isMetaRefresh checks if a <meta> element is a http-equiv="refresh".
func isMetaRefresh(n *html.Node) bool {
	return strings.EqualFold(getAttrValue(n, "http-equiv"), "refresh")
}

// isJSRefresh checks if a meta refresh redirects via javascript:.
func isJSRefresh(n *html.Node) bool {
	content := getAttrValue(n, "content")
	if content == "" {
		return false
	}
	return strings.Contains(strings.ToLower(content), "javascript:")
}

// ---------------------------------------------------------------------------
// Charset, viewport, and mobile CSS injection.
// ---------------------------------------------------------------------------

// ensureCharset guarantees a <meta charset="utf-8"> in the document head.
func ensureCharset(doc *html.Node, report *CleanReport) {
	head := findElement(doc, atom.Head)
	if head == nil {
		return
	}

	// Check if charset is already declared.
	for c := head.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.DataAtom != atom.Meta {
			continue
		}
		if getAttrValue(c, "charset") != "" {
			return
		}
		if strings.EqualFold(getAttrValue(c, "http-equiv"), "content-type") &&
			strings.Contains(strings.ToLower(getAttrValue(c, "content")), "charset=") {
			return
		}
	}

	// Insert charset meta at the head's beginning.
	meta := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Meta,
		Data:     "meta",
		Attr:     []html.Attribute{{Key: "charset", Val: "utf-8"}},
	}
	if head.FirstChild != nil {
		head.InsertBefore(meta, head.FirstChild)
	} else {
		head.AppendChild(meta)
	}
	report.CharsetAdded = true
}

// ensureViewport inserts a viewport meta tag for mobile devices.
func ensureViewport(doc *html.Node) {
	head := findElement(doc, atom.Head)
	if head == nil {
		return
	}

	// Check if viewport already exists.
	for c := head.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == atom.Meta &&
			strings.EqualFold(getAttrValue(c, "name"), "viewport") {
			return
		}
	}

	vp := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Meta,
		Data:     "meta",
		Attr: []html.Attribute{
			{Key: "name", Val: "viewport"},
			{Key: "content", Val: "width=device-width, initial-scale=1"},
		},
	}
	if head.FirstChild != nil {
		head.InsertBefore(vp, head.FirstChild)
	} else {
		head.AppendChild(vp)
	}
}

// mobileCSS contains CSS rules to make old table-layout sites readable
// on mobile devices.
const mobileCSS = `
*, *:before, *:after {
  box-sizing: border-box;
}
body {
  font-size: 18px;
  line-height: 1.6;
  word-wrap: break-word;
  overflow-wrap: break-word;
}
font {
  font-size: inherit !important;
  font-family: inherit !important;
  color: inherit !important;
}
*[width], *[height] {
  max-width: 100% !important;
  height: auto !important;
}
table {
  max-width: 100% !important;
  display: block;
  overflow-x: auto;
}
td, th {
  word-break: break-word;
}
img {
  max-width: 100% !important;
  height: auto !important;
}
map, area[shape] {
  display: none;
}
td:has(img[usemap]) {
  display: none;
}
`

// injectMobileCSS adds mobile-friendly CSS to the document head.
func injectMobileCSS(doc *html.Node) {
	head := findElement(doc, atom.Head)
	if head == nil {
		return
	}

	style := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Style,
		Data:     "style",
	}
	style.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: mobileCSS,
	})
	head.AppendChild(style)
}

// insertBanner adds an HTML comment at the document's top.
func insertBanner(doc *html.Node, banner string) {
	bannerNode := &html.Node{
		Type: html.CommentNode,
		Data: " " + banner + " ",
	}
	if doc.FirstChild != nil {
		doc.InsertBefore(bannerNode, doc.FirstChild)
	} else {
		doc.AppendChild(bannerNode)
	}
}

// ---------------------------------------------------------------------------
// Helper functions.
// ---------------------------------------------------------------------------

// getAttrValue returns the value of an attribute, case-insensitive.
func getAttrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// findElement finds the first element with a given tag name.
func findElement(n *html.Node, tag atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fallback: regex-based cleaning (used when DOM parsing fails).
// ---------------------------------------------------------------------------

func cleanHTMLRegex(htmlStr string, report *CleanReport) string {
	s := htmlStr

	// Count scripts before removal.
	report.ScriptsRemoved = countMatches(s, `<script`)

	s = removeScriptTags(s)
	s = removeInlineEvents(s)
	s = removeNoScriptTags(s)
	s = removeIframeTags(s)
	s = removeEmbedTags(s)
	s = removeBaseTag(s)
	s = removeMetaRefresh(s)

	return s
}

func countMatches(s, substr string) int {
	count := 0
	lower := strings.ToLower(s)
	for {
		i := strings.Index(lower, substr)
		if i < 0 {
			break
		}
		count++
		lower = lower[i+len(substr):]
	}
	return count
}
