// Package antibot provides automatic anti-bot detection and response
// for website cloning. It detects common blocking patterns (HTTP status
// codes, Cloudflare challenges, CAPTCHA pages) and triggers escalation
// of stealth measures through five configurable levels.
package antibot

import (
	"net/http"
	"strings"
)

// BlockReason categorises the type of blocking detected.
type BlockReason string

const (
	// ReasonNone indicates no blocking was detected.
	ReasonNone BlockReason = ""

	// ReasonForbidden is HTTP 403 — server explicitly denied access.
	ReasonForbidden BlockReason = "forbidden"

	// ReasonRateLimited is HTTP 429 — too many requests.
	ReasonRateLimited BlockReason = "rate_limited"

	// ReasonUnavailable is HTTP 503 — service temporarily unavailable.
	ReasonUnavailable BlockReason = "unavailable"

	// ReasonCloudflare indicates a Cloudflare challenge page (cf-ray header).
	ReasonCloudflare BlockReason = "cloudflare"

	// ReasonCaptcha indicates a CAPTCHA/verification page in the DOM.
	ReasonCaptcha BlockReason = "captcha"

	// ReasonBlocked indicates generic "access denied" / "blocked" content.
	ReasonBlocked BlockReason = "blocked"

	// ReasonTimeout indicates the page navigation timed out (possible bot wall).
	ReasonTimeout BlockReason = "timeout"

	// ReasonEmpty indicates the page returned empty content (suspicious).
	ReasonEmpty BlockReason = "empty"
)

// captchaKeywords are strings commonly found in anti-bot / CAPTCHA pages.
var captchaKeywords = []string{
	"captcha",
	"verify you are human",
	"verify you are a human",
	"are you a human",
	"please verify",
	"security check",
	"challenge",
	"cf-challenge",
	"cf_challenge",
	"cf-browser-verification",
	"checking your browser",
	"ddos protection",
	"access denied",
	"blocked",
	"your request has been blocked",
	"please enable javascript",
	"javascript is required",
	"browser check",
	"automated access",
	"suspicious activity",
}

// cloudflareHeaders are response headers set by Cloudflare anti-bot.
var cloudflareHeaders = []string{
	"cf-ray",
	"cf-chl-bypass",
	"cf-chl-out",
	"cf-chl-proxied",
	"cf-cache-status",
	"cf-connecting-ip",
	"cf-ipcountry",
	"cf-visitor",
	"x-sucuri-id",
	"x-sucuri-cache",
	"x-iinfo",
	"x-cdn",
}

// DetectHTTP analyses an HTTP response for anti-bot indicators.
// Returns the detected reason and a human-readable description.
func DetectHTTP(statusCode int, headers http.Header) (BlockReason, string) {
	switch statusCode {
	case 403:
		return ReasonForbidden,
			"HTTP 403 Forbidden — server denied access, " +
				"likely anti-bot protection"

	case 429:
		return ReasonRateLimited,
			"HTTP 429 Too Many Requests — rate limit exceeded"

	case 503:
		// Check for Cloudflare-specific headers.
		for _, h := range cloudflareHeaders {
			if headers.Get(h) != "" {
				return ReasonCloudflare,
					"HTTP 503 + Cloudflare headers — " +
						"anti-bot challenge page"
			}
		}
		return ReasonUnavailable,
			"HTTP 503 Service Unavailable — possible bot wall"
	}

	// Check Cloudflare headers even on 200 (challenge-passed pages).
	for _, h := range cloudflareHeaders {
		if headers.Get(h) != "" {
			return ReasonCloudflare,
				"Cloudflare headers detected — site uses anti-bot"
		}
	}

	return ReasonNone, ""
}

// DetectDOM analyses HTML content for anti-bot / CAPTCHA indicators.
// Returns the detected reason and a snippet of the matching text.
func DetectDOM(html string) (BlockReason, string) {
	if len(html) == 0 {
		return ReasonEmpty, "empty response body"
	}

	lower := strings.ToLower(html)

	// Check for very short pages (likely error/block pages).
	if len(html) < 200 {
		if strings.Contains(lower, "forbidden") ||
			strings.Contains(lower, "denied") {
			return ReasonBlocked, "short blocked page"
		}
	}

	// Check for CAPTCHA / anti-bot keywords.
	for _, kw := range captchaKeywords {
		idx := strings.Index(lower, kw)
		if idx >= 0 {
			// Extract a snippet around the keyword.
			snippet := extractSnippet(html, idx, len(kw), 80)
			return ReasonCaptcha,
				"anti-bot page detected: " + snippet
		}
	}

	return ReasonNone, ""
}

// Detect analyses both HTTP response and page content.
// Returns the most specific blocking reason detected.
func Detect(statusCode int, headers http.Header,
	htmlContent string) (BlockReason, string) {
	// Check HTTP-level first (fastest and most reliable).
	reason, desc := DetectHTTP(statusCode, headers)
	if reason != ReasonNone {
		return reason, desc
	}

	// Check DOM content for text-based anti-bot patterns.
	reason, desc = DetectDOM(htmlContent)
	if reason != ReasonNone {
		return reason, desc
	}

	return ReasonNone, ""
}

// IsBlocked returns true if the given error reason indicates blocking.
func IsBlocked(reason BlockReason) bool {
	return reason != ReasonNone && reason != ReasonTimeout
}

// ShouldRetry returns true if this block type typically can be bypassed
// with stronger stealth measures.
func ShouldRetry(reason BlockReason) bool {
	switch reason {
	case ReasonForbidden, ReasonRateLimited, ReasonCloudflare,
		ReasonCaptcha, ReasonBlocked, ReasonTimeout:
		return true
	case ReasonUnavailable:
		return true // 503 may resolve with retry
	case ReasonEmpty, ReasonNone:
		return false
	default:
		return true
	}
}

// extractSnippet returns a short context window around text position.
func extractSnippet(html string, pos, kwLen, window int) string {
	start := max(pos-window, 0)
	end := min(pos+kwLen+window, len(html))
	snippet := strings.TrimSpace(html[start:end])
	// Replace newlines for single-line display.
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	// Collapse whitespace.
	snippet = strings.Join(strings.Fields(snippet), " ")
	if len(snippet) > 120 {
		snippet = snippet[:120] + "..."
	}
	return snippet
}
