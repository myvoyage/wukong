// Package clone provides website cloning functionality.
//
// css.go: CSS url() and @import rewriting module.
// During offline archiving, CSS files often reference external resources
// (images, fonts, other stylesheets) via url() and @import. This module
// extracts those references, resolves them, and rewrites them to point
// to local paths within the mirror.
package clone

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// cssURLRe matches url("..."), url('...'), and url(...) patterns in CSS.
// It captures the quoted/unquoted URL value.
var cssURLRe = regexp.MustCompile(
	`url\(\s*(?:"([^"]*)"|'([^']*)'|([^'")\s]+))\s*\)`)

// cssImportRe matches @import "..." or @import '...' in CSS.
// Note: @import url(...) is already handled by cssURLRe.
var cssImportRe = regexp.MustCompile(
	`@import\s+(?:"([^"]*)"|'([^']*)')`)

// AssetRefHandler is called for each asset reference found in CSS.
// It receives the absolute URL of the referenced asset and returns
// the local path to use in the rewritten CSS. Return empty string
// to skip rewriting (keep original).
type AssetRefHandler func(absURL string) (localPath string)

// RewriteCSS rewrites url() and @import references in CSS content.
//   - cssContent: the raw CSS bytes.
//   - cssBaseURL: the absolute URL of the CSS file itself (used to resolve relative refs).
//   - handler: called for each asset reference; returns local path.
//
// Returns the rewritten CSS content.
func RewriteCSS(cssContent []byte, cssBaseURL string, handler AssetRefHandler) []byte {
	s := string(cssContent)

	// Rewrite @import statements.
	s = cssImportRe.ReplaceAllStringFunc(s, func(match string) string {
		groups := cssImportRe.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		ref := groups[1]
		if ref == "" {
			ref = groups[2]
		}
		return rewriteCSSRef(match, ref, cssBaseURL, "@import \"%s\"", handler)
	})

	// Rewrite url() references.
	s = cssURLRe.ReplaceAllStringFunc(s, func(match string) string {
		groups := cssURLRe.FindStringSubmatch(match)
		if len(groups) < 4 {
			return match
		}
		ref := groups[1]
		if ref == "" {
			ref = groups[2]
		}
		if ref == "" {
			ref = groups[3]
		}
		return rewriteCSSRef(match, ref, cssBaseURL, "url(\"%s\")", handler)
	})

	return []byte(s)
}

// rewriteCSSRef resolves a single CSS reference and produces the rewritten form.
func rewriteCSSRef(match, ref, cssBaseURL, format string, handler AssetRefHandler) string {
	// Skip empty, data: URIs, and fragment-only refs.
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return match
	}
	if strings.HasPrefix(strings.ToLower(ref), "data:") {
		return match
	}

	// Resolve relative to CSS file's own URL.
	absURL, err := resolveCSSRef(cssBaseURL, ref)
	if err != nil || absURL == "" {
		return match
	}

	// Call handler to get local path.
	localPath := handler(absURL)
	if localPath == "" {
		return match // Handler chose to skip.
	}

	return fmt.Sprintf(format, localPath)
}

// resolveCSSRef resolves a CSS reference (relative or absolute) against
// the CSS file's base URL to produce an absolute URL.
func resolveCSSRef(cssBaseURL, ref string) (string, error) {
	base, err := url.Parse(cssBaseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}

	refURL, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parse ref URL: %w", err)
	}

	resolved := base.ResolveReference(refURL)

	// Reject non-fetchable schemes.
	switch resolved.Scheme {
	case "http", "https":
		return resolved.String(), nil
	case "data":
		return "", nil // Skip data URIs silently.
	default:
		return "", fmt.Errorf("unsupported scheme: %s", resolved.Scheme)
	}
}

// ExtractCSSAssetRefs extracts all asset URL references from CSS content
// for discovery and download queuing.
func ExtractCSSAssetRefs(cssContent []byte, cssBaseURL string) []string {
	var urls []string
	seen := make(map[string]bool)

	// Collect from @import.
	s := string(cssContent)
	for _, match := range cssImportRe.FindAllStringSubmatch(s, -1) {
		ref := match[1]
		if ref == "" {
			ref = match[2]
		}
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasPrefix(ref, "#") ||
			strings.HasPrefix(strings.ToLower(ref), "data:") {
			continue
		}
		if absURL, err := resolveCSSRef(cssBaseURL, ref); err == nil && absURL != "" {
			if !seen[absURL] {
				seen[absURL] = true
				urls = append(urls, absURL)
			}
		}
	}

	// Collect from url().
	for _, groups := range cssURLRe.FindAllStringSubmatch(s, -1) {
		ref := groups[1]
		if ref == "" {
			ref = groups[2]
		}
		if ref == "" {
			ref = groups[3]
		}
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasPrefix(ref, "#") ||
			strings.HasPrefix(strings.ToLower(ref), "data:") {
			continue
		}
		if absURL, err := resolveCSSRef(cssBaseURL, ref); err == nil && absURL != "" {
			if !seen[absURL] {
				seen[absURL] = true
				urls = append(urls, absURL)
			}
		}
	}

	return urls
}

// DiscoverCSSAssets extracts all asset URLs from a CSS file and enqueues them
// if not already handled. It returns the list of newly discovered URLs.
func DiscoverCSSAssets(cssContent []byte, cssBaseURL string,
	alreadySeen func(string) bool) []string {
	refs := ExtractCSSAssetRefs(cssContent, cssBaseURL)
	var newURLs []string
	for _, u := range refs {
		if alreadySeen != nil && alreadySeen(u) {
			continue
		}
		newURLs = append(newURLs, u)
	}
	return newURLs
}
