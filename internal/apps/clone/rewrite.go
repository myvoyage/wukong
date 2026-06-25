// Package clone provides website cloning functionality.
//
// rewrite.go: DOM-based HTML link and asset reference rewriting.
// Walks the parsed HTML tree and replaces all external URL references
// (href, src, srcset, poster, data, etc.) with local paths.
// Detects page links vs asset links automatically and delegates
// path generation via a callback (RewriteSink).
package clone

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// RewriteSink receives an absolute URL and its resource kind, and returns
// the local path to use in the rewritten HTML. Return "" to keep the
// original URL unchanged (e.g., for external links).
type RewriteSink func(absURL string, kind URLKind) (localPath string)

// RewriteHTML walks the parsed HTML DOM tree and rewrites all external
// URL references to local paths using the provided sink callback.
//
// Supported elements and attributes:
//   <a href>, <area href>          → page/asset (based on LikelyPage)
//   <link href>                    → asset (stylesheet, icon, preload, etc.)
//   <img src/srcset>               → asset
//   <source src/srcset>            → asset
//   <video src/poster>             → asset
//   <audio src>, <track src>       → asset
//   <embed src>, <object data>     → asset
//   <iframe src>, <frame src>      → page/asset
//   <script src>                   → asset (kept if already local)
//   <style> text content           → CSS url() rewriting
//   style="" attribute              → inline CSS url() rewriting
func RewriteHTML(root *html.Node, base *url.URL, sink RewriteSink) {
	walkAndRewrite(root, base, sink)
}

// walkAndRewrite recursively traverses the DOM and rewrites each element.
func walkAndRewrite(n *html.Node, base *url.URL, sink RewriteSink) {
	if n.Type == html.ElementNode {
		rewriteElement(n, base, sink)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkAndRewrite(c, base, sink)
	}
}

// rewriteElement dispatches to the appropriate rewriter based on element tag.
func rewriteElement(n *html.Node, base *url.URL, sink RewriteSink) {
	switch n.DataAtom {
	// Page-or-Asset elements.
	case atom.A, atom.Area:
		rewriteAttr(n, base, "href", sink, pageOrAssetKind)
	case atom.Iframe, atom.Frame:
		rewriteAttr(n, base, "src", sink, pageOrAssetKind)

	// Asset-only elements.
	case atom.Link:
		rewriteLink(n, base, sink)
	case atom.Img:
		rewriteAttr(n, base, "src", sink, alwaysAsset)
		rewriteSrcset(n, base, sink)
	case atom.Source:
		rewriteAttr(n, base, "src", sink, alwaysAsset)
		rewriteSrcset(n, base, sink)
	case atom.Video:
		rewriteAttr(n, base, "src", sink, alwaysAsset)
		rewriteAttr(n, base, "poster", sink, alwaysAsset)
	case atom.Audio:
		rewriteAttr(n, base, "src", sink, alwaysAsset)
	case atom.Track:
		rewriteAttr(n, base, "src", sink, alwaysAsset)
	case atom.Embed:
		rewriteAttr(n, base, "src", sink, alwaysAsset)
	case atom.Object:
		rewriteAttr(n, base, "data", sink, alwaysAsset)
	case atom.Script:
		rewriteAttr(n, base, "src", sink, alwaysAsset)

	// CSS rewriting.
	case atom.Style:
		rewriteStyleElement(n, base, sink)
	}

	// Inline style attribute on any element.
	rewriteInlineStyle(n, base, sink)
}

// ---------------------------------------------------------------------------
// Attribute rewriting.
// ---------------------------------------------------------------------------

// kindDecider returns the URLKind for a given absolute URL.
type kindDecider func(absURL string) URLKind

func alwaysAsset(absURL string) URLKind  { return KindAsset }
func pageOrAssetKind(absURL string) URLKind {
	if LikelyPage(absURL) {
		return KindPage
	}
	return KindAsset
}

// rewriteAttr finds an attribute by name (case-insensitive), normalizes its
// value, and replaces it with the result of the sink callback.
func rewriteAttr(n *html.Node, base *url.URL, attrName string, sink RewriteSink, decide kindDecider) {
	for i, a := range n.Attr {
		if !strings.EqualFold(a.Key, attrName) {
			continue
		}

		val := strings.TrimSpace(a.Val)
		if val == "" {
			return
		}

		// Skip non-fetchable values.
		if shouldSkipRef(val) {
			return
		}

		// Normalize the reference.
		absURL, err := Normalize(base.String(), val)
		if err != nil || absURL == "" {
			return
		}

		kind := decide(absURL)
		localPath := sink(absURL, kind)
		if localPath == "" {
			return // Sink chose to keep original.
		}

		n.Attr[i].Val = localPath
		return
	}
}

// rewriteSrcset handles the srcset attribute which contains comma-separated
// URL-descriptor pairs like "img.jpg 1x, img2x.jpg 2x".
func rewriteSrcset(n *html.Node, base *url.URL, sink RewriteSink) {
	for i, a := range n.Attr {
		if !strings.EqualFold(a.Key, "srcset") {
			continue
		}

		parts := strings.Split(a.Val, ",")
		var rewritten []string

		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			fields := strings.Fields(part)
			if len(fields) == 0 {
				continue
			}

			urlStr := fields[0]
			if shouldSkipRef(urlStr) {
				rewritten = append(rewritten, part)
				continue
			}

			absURL, err := Normalize(base.String(), urlStr)
			if err != nil || absURL == "" {
				rewritten = append(rewritten, part)
				continue
			}

			localPath := sink(absURL, KindAsset)
			if localPath == "" {
				rewritten = append(rewritten, part)
				continue
			}

			fields[0] = localPath
			rewritten = append(rewritten, strings.Join(fields, " "))
		}

		n.Attr[i].Val = strings.Join(rewritten, ", ")
		return
	}
}

// ---------------------------------------------------------------------------
// <link> element rewriting.
// ---------------------------------------------------------------------------

// assetRels is the set of <link rel> values that indicate an asset reference.
var assetRels = map[string]bool{
	"stylesheet":                   true,
	"icon":                         true,
	"apple-touch-icon":             true,
	"apple-touch-icon-precomposed": true,
	"mask-icon":                    true,
	"manifest":                     true,
	"preload":                      true,
	"prefetch":                     true,
}

// rewriteLink rewrites the href attribute of a <link> element only if
// its rel attribute indicates an asset (stylesheet, icon, etc.).
func rewriteLink(n *html.Node, base *url.URL, sink RewriteSink) {
	// Check if rel indicates a downloadable resource.
	rel := getAttrCI(n, "rel")
	if rel == "" {
		return
	}

	tokens := strings.Fields(strings.ToLower(rel))
	isAsset := false
	for _, t := range tokens {
		if assetRels[t] {
			isAsset = true
			break
		}
	}
	if !isAsset {
		return
	}

	rewriteAttr(n, base, "href", sink, alwaysAsset)
}

// ---------------------------------------------------------------------------
// CSS rewriting within HTML.
// ---------------------------------------------------------------------------

// rewriteStyleElement rewrites url() references inside a <style> element.
func rewriteStyleElement(n *html.Node, base *url.URL, sink RewriteSink) {
	if n.FirstChild == nil || n.FirstChild.Type != html.TextNode {
		return
	}

	cssText := n.FirstChild.Data
	if cssText == "" {
		return
	}

	rewritten := RewriteCSS([]byte(cssText), base.String(), func(absURL string) string {
		return sink(absURL, KindAsset)
	})

	n.FirstChild.Data = string(rewritten)
}

// rewriteInlineStyle rewrites url() references in an element's style attribute.
func rewriteInlineStyle(n *html.Node, base *url.URL, sink RewriteSink) {
	for i, a := range n.Attr {
		if !strings.EqualFold(a.Key, "style") {
			continue
		}
		if a.Val == "" {
			return
		}

		// Wrap style value as a CSS rule with a dummy selector to use RewriteCSS.
		css := fmt.Sprintf("x{%s}", a.Val)
		rewritten := RewriteCSS([]byte(css), base.String(), func(absURL string) string {
			return sink(absURL, KindAsset)
		})

		// Unwrap: strip the "x{" prefix and "}" suffix.
		result := string(rewritten)
		result = strings.TrimPrefix(result, "x{")
		result = strings.TrimSuffix(result, "}")
		n.Attr[i].Val = strings.TrimSpace(result)
		return
	}
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// shouldSkipRef returns true for URL values that should NOT be rewritten.
func shouldSkipRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return true
	}
	lower := strings.ToLower(ref)
	return strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "data:")
}

// getAttrCI returns an attribute's value with case-insensitive key lookup.
func getAttrCI(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}
