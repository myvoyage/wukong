// Package browser provides headless Chrome browser pool management
// for website cloning.
//
// Manages a single Chrome browser instance with a semaphore-controlled
// tab pool for concurrent page rendering.
package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/km269/wukong/internal/browser/settle"
)

// Pool manages a pool of headless Chrome tabs for concurrent page rendering.
// A single Chrome browser process is shared; tabs are created on demand
// and limited by a semaphore.
type Pool struct {
	opts           PoolOptions
	allocCtx       context.Context // Allocator context (for resource cleanup).
	allocCancel    context.CancelFunc
	browserCtx     context.Context // Browser context (single browser instance).
	browserCancel  context.CancelFunc
	sem            chan struct{}   // Semaphore for concurrency control.
	mu             sync.Mutex
	closed         bool
	stealthEnabled bool            // True if stealth is currently active.
	initOnce       sync.Once
	initErr        error
}

// PoolOptions configures the browser pool.
type PoolOptions struct {
	Headless      bool
	Workers       int
	Settle        time.Duration
	RenderTimeout time.Duration
	Scroll        bool
	Stealth       bool // Inject anti-detection script before page loads.
	ChromePath    string
}

// DefaultPoolOptions returns reasonable defaults.
func DefaultPoolOptions() PoolOptions {
	return PoolOptions{
		Headless:      true,
		Workers:       4,
		Settle:        1500 * time.Millisecond,
		RenderTimeout: 60 * time.Second,
		Scroll:        false,
		Stealth:       false,
		ChromePath:    "",
	}
}

// RenderResult holds the output of a single page render.
type RenderResult struct {
	HTML        string
	Title       string
	FinalURL    string
	ContentType string
}

// NewPool creates a new browser pool.
func NewPool(opts PoolOptions) *Pool {
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.Settle <= 0 {
		opts.Settle = 1500 * time.Millisecond
	}
	if opts.RenderTimeout <= 0 {
		opts.RenderTimeout = 60 * time.Second
	}

	return &Pool{
		opts: opts,
		sem:  make(chan struct{}, opts.Workers),
	}
}

// initBrowser lazily starts the Chrome browser on first use.
func (p *Pool) initBrowser() {
	p.initOnce.Do(func() {
		chromePath := p.opts.ChromePath
		if chromePath == "" {
			chromePath = findChrome()
		}

		if chromePath != "" {
			fmt.Fprintf(os.Stderr, "[wukong/browser] using Chrome: %s\n", chromePath)
		} else {
			fmt.Fprintf(os.Stderr, "[wukong/browser] Chrome not found in known paths, using auto-detection\n")
		}

		allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", p.opts.Headless),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("mute-audio", true),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("disable-translate", true),
			chromedp.Flag("disable-popup-blocking", true),
		)

		// Stealth mode: add anti-detection flags for realistic fingerprint.
		if p.opts.Stealth {
			allocOpts = append(allocOpts,
				chromedp.Flag("disable-blink-features", "AutomationControlled"),
				chromedp.Flag("disable-infobars", true),
				chromedp.Flag("no-default-browser-check", true),
				chromedp.Flag("no-first-run", true),
				chromedp.Flag("disable-component-update", true),
				chromedp.Flag("window-size", "1920,1080"),
				chromedp.Flag("disable-breakpad", true),
				chromedp.Flag("disable-background-timer-throttling", true),
				chromedp.Flag("disable-renderer-backgrounding", true),
				chromedp.Flag("disable-field-trial-config", true),
				chromedp.Flag("force-color-profile", "srgb"),
			)
		}

		if isContainer() || isRoot() {
			allocOpts = append(allocOpts, chromedp.Flag("no-sandbox", true))
			fmt.Fprintf(os.Stderr, "[wukong/browser] container/root detected, sandbox disabled\n")
		}

		if chromePath != "" {
			allocOpts = append(allocOpts, chromedp.ExecPath(chromePath))
		}

		// Create allocator context → browser context chain.
		p.allocCtx, p.allocCancel = chromedp.NewExecAllocator(
			context.Background(), allocOpts...)
		p.browserCtx, p.browserCancel = chromedp.NewContext(p.allocCtx)

		// Verify the browser launches by opening a blank page.
		fmt.Fprintf(os.Stderr, "[wukong/browser] starting headless Chrome...\n")
		if err := chromedp.Run(p.browserCtx, chromedp.Navigate("about:blank")); err != nil {
			p.initErr = fmt.Errorf("browser startup failed: %w\n\n"+
				"Chrome/Chromium is required for website cloning.\n"+
				"Install from: https://www.google.com/chrome/\n"+
				"Or set the path: wukong apps clone --chrome-path <path>", err)
			p.browserCancel()
			p.allocCancel()
		} else {
			// Inject stealth anti-detection script if enabled.
			if p.opts.Stealth {
				if err := chromedp.Run(p.browserCtx, injectStealthAction()); err != nil {
					fmt.Fprintf(os.Stderr,
						"[wukong/browser] stealth injection failed: %v\n", err)
				} else {
					p.stealthEnabled = true
					fmt.Fprintf(os.Stderr,
						"[wukong/browser] stealth mode enabled\n")
				}
			}
			fmt.Fprintf(os.Stderr, "[wukong/browser] ready\n")
		}
	})
}

// Render loads a URL in a headless Chrome tab, waits for it to settle,
// and snapshots the final DOM as HTML.
func (p *Pool) Render(ctx context.Context, rawURL string) (*RenderResult, error) {
	// Acquire semaphore slot.
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Initialize browser on first use.
	p.initBrowser()
	if p.initErr != nil {
		return nil, p.initErr
	}

	// Create a new tab from the shared browser context.
	tabCtx, tabCancel := chromedp.NewContext(p.browserCtx)
	defer tabCancel()

	// Apply per-page timeout to the tab context.
	tabCtx, tabTimeoutCancel := context.WithTimeout(tabCtx, p.opts.RenderTimeout)
	defer tabTimeoutCancel()

	// Listen for main document response to detect non-HTML content.
	var contentType string
	chromedp.ListenTarget(tabCtx, func(v any) {
		if ev, ok := v.(*network.EventResponseReceived); ok {
			if ev.Type == network.ResourceTypeDocument && contentType == "" {
				contentType = ev.Response.MimeType
			}
		}
	})

	// Navigate to the URL. All Run calls use tabCtx (or derived).
	if err := chromedp.Run(tabCtx, chromedp.Navigate(rawURL)); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Check for non-HTML response.
	if contentType != "" && !isHTMLContentType(contentType) {
		return nil, &ErrNotHTML{URL: rawURL, ContentType: contentType}
	}

	// Wait for network to settle (idle for Settle duration).
	// This replaces fixed Sleep with network-activity-based waiting.
	if err := p.waitForNetworkSettle(tabCtx); err != nil {
		// Non-fatal: continue even if settle times out.
	}

	// Optional scroll to trigger lazy loading.
	if p.opts.Scroll {
		p.autoScroll(tabCtx)
		// Wait for network to settle again after scroll-triggered loading.
		p.waitForNetworkSettle(tabCtx)
	}

	// Snapshot the DOM.
	var htmlContent, title, finalURL string
	snapshotCtx, snapshotCancel := context.WithTimeout(tabCtx, 5*time.Second)
	err := chromedp.Run(snapshotCtx,
		chromedp.Title(&title),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &htmlContent),
	)
	snapshotCancel()
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}

	return &RenderResult{
		HTML:        htmlContent,
		Title:       title,
		FinalURL:    finalURL,
		ContentType: contentType,
	}, nil
}

// StealthEnabled reports whether anti-detection scripts are currently active.
func (p *Pool) StealthEnabled() bool {
	return p.stealthEnabled
}

// EnableStealth dynamically injects anti-detection scripts into the running
// browser pool. This is called by the anti-bot engine when blocking is
// detected mid-crawl, allowing stealth to be applied without restarting
// the browser. Uses Page.addScriptToEvaluateOnNewDocument so the script
// takes effect for all subsequent page loads.
func (p *Pool) EnableStealth() error {
	if p.stealthEnabled {
		return nil // Already active.
	}

	p.initBrowser()
	if p.initErr != nil {
		return p.initErr
	}

	if err := chromedp.Run(p.browserCtx, injectStealthAction()); err != nil {
		return fmt.Errorf("enable stealth: %w", err)
	}
	p.stealthEnabled = true
	fmt.Fprintf(os.Stderr,
		"[wukong/browser] stealth dynamically enabled via anti-bot escalation\n")
	return nil
}

// Close gracefully shuts down the browser pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true

	if p.browserCancel != nil {
		p.browserCancel()
	}
	if p.allocCancel != nil {
		p.allocCancel()
	}
}

// waitForNetworkSettle delegates to the shared network-idle wait strategy.
func (p *Pool) waitForNetworkSettle(tabCtx context.Context) error {
	return settle.Wait(tabCtx, p.opts.Settle)
}

// autoScroll scrolls to the bottom to trigger lazy loading, then back to top.
func (p *Pool) autoScroll(tabCtx context.Context) {
	// Use a short timeout; scrolling is best-effort.
	scrollCtx, cancel := context.WithTimeout(tabCtx, 5*time.Second)
	defer cancel()

	chromedp.Run(scrollCtx,
		chromedp.Evaluate(`(function() {
			var h = document.body.scrollHeight;
			var s = window.innerHeight;
			var pos = 0;
			function step() {
				pos += s;
				if (pos >= h) { window.scrollTo(0,0); return; }
				window.scrollTo(0, pos);
				setTimeout(step, 300);
			}
			step();
		})()`, nil),
	)
}

// ---------------------------------------------------------------------------
// Error types.
// ---------------------------------------------------------------------------

// ErrNotHTML indicates the server returned a non-HTML response.
type ErrNotHTML struct {
	URL         string
	ContentType string
}

func (e *ErrNotHTML) Error() string {
	return fmt.Sprintf("page %s returned non-HTML content type: %s",
		e.URL, e.ContentType)
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

func isHTMLContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return true
	}
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	return ct == "text/html" || ct == "application/xhtml+xml"
}

func findChrome() string {
	if p := os.Getenv("KAGE_CHROME"); p != "" {
		return p
	}
	if p := os.Getenv("CHROME_BIN"); p != "" {
		return p
	}
	if p := os.Getenv("CHROMIUM_BIN"); p != "" {
		return p
	}

	candidates := []string{
		"google-chrome", "google-chrome-stable", "chromium",
		"chromium-browser", "chrome", "chrome.exe",
		"/usr/bin/google-chrome", "/usr/bin/chromium",
		"/usr/bin/chromium-browser", "/usr/bin/google-chrome-stable",
		"/snap/bin/chromium",
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
		os.ExpandEnv("$LOCALAPPDATA\\Google\\Chrome\\Application\\chrome.exe"),
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func isContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil && (strings.Contains(string(data), "docker") ||
		strings.Contains(string(data), "kubepods") ||
		strings.Contains(string(data), "containerd")) {
		return true
	}
	return false
}

func isRoot() bool {
	return os.Getuid() == 0
}
