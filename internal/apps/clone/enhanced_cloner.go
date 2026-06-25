// Package clone provides website cloning functionality.
//
// enhanced_cloner.go: Improved clone engine integrating browser pool,
// frontier-based resume, robots.txt compliance, sitemap discovery,
// rate limiting, content deduplication, and CSS rewriting.
package clone

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/km269/wukong/internal/apps/browser"
	"github.com/km269/wukong/internal/apps/sanitize"
	"github.com/km269/wukong/internal/browser/antibot"
	"golang.org/x/net/html"
)

// TraversalMode defines the crawl traversal strategy.
type TraversalMode string

const (
	// TraversalBFS uses breadth-first search: pages are crawled layer by layer.
	TraversalBFS TraversalMode = "bfs"
	// TraversalDFS uses depth-first search: each branch is followed to its
	// maximum depth before backtracking.
	TraversalDFS TraversalMode = "dfs"
)

// EnhancedClonerOptions configures the enhanced cloning engine.
type EnhancedClonerOptions struct {
	// OutputDir is the base directory for cloned output.
	OutputDir string

	// MaxPages limits the total number of pages to clone (0 = unlimited).
	MaxPages int

	// MaxDepth limits the link depth from the seed URL (0 = unlimited).
	MaxDepth int

	// Traversal sets the crawl strategy: "bfs" (breadth-first) or "dfs"
	// (depth-first). Default is "bfs".
	Traversal TraversalMode

	// Subdomains includes subdomains of the seed host in scope.
	Subdomains bool

	// Scroll enables auto-scrolling to trigger lazy loading.
	Scroll bool

	// ScopePrefix restricts crawling to paths starting with this prefix.
	ScopePrefix string

	// Exclude lists path prefixes to skip.
	Exclude []string

	// Refresh re-renders all pages even if they exist locally.
	Refresh bool

	// Force deletes existing clone data and starts fresh.
	Force bool

	// Workers is the number of concurrent page renderers.
	Workers int

	// AssetWorkers is the number of concurrent asset downloaders.
	AssetWorkers int

	// RespectRobots controls whether to obey robots.txt rules.
	RespectRobots bool

	// CrawlDelay overrides the robots.txt crawl-delay.
	CrawlDelay time.Duration

	// NoSitemap disables sitemap-based URL discovery.
	NoSitemap bool

	// EnableResume saves frontier state for resuming interrupted crawls.
	EnableResume bool

	// DedupContent enables SHA-256 content deduplication with hard links.
	DedupContent bool

	// MobileReadable injects responsive CSS for mobile viewing.
	MobileReadable bool

	// Stealth injects anti-detection scripts to hide automation.
	Stealth bool

	// Incremental enables ETag/Last-Modified caching to skip unchanged pages.
	Incremental bool

	// CacheMaxAge is the maximum age of cached content before it's considered
	// stale. Only used when Incremental is true. Default is 24 hours.
	CacheMaxAge time.Duration

	// Timeout is the per-page rendering timeout.
	Timeout time.Duration

	// Settle is the network-idle quiet period before snapshotting the DOM.
	// Default is 1500ms.
	Settle time.Duration

	// ChromePath is the path to the Chrome/Chromium executable.
	ChromePath string

	// UserAgent is the User-Agent header for HTTP requests.
	UserAgent string

	// AssetSameDomain only downloads assets from the same registrable domain
	// as the seed URL. Assets on external domains (CDNs, analytics) are left
	// as live links. Default is true.
	AssetSameDomain bool

	// SkipAssetExts is a set of file extensions that should NOT be downloaded.
	// Assets matching these extensions keep their original remote URLs.
	// Typical values: .mp4, .pdf, .zip, .exe, etc.
	SkipAssetExts map[string]bool

	// MaxAssetBytes is the maximum size in bytes for a single asset download.
	// Assets exceeding this limit are left as live links. Default is 50MB.
	MaxAssetBytes int64

	// AntibotEnabled enables automatic anti-bot detection and response.
	// When a page or asset is blocked (403, Cloudflare challenge, CAPTCHA),
	// the engine detects the pattern and auto-escalates stealth measures.
	// Default is true.
	AntibotEnabled bool

	// AntibotAutoEscalate enables automatic escalation on detection.
	// When false, blocks are detected and logged but the engine does NOT
	// change stealth levels automatically. Default is true.
	AntibotAutoEscalate bool

	// CookieFile is the path to a Netscape-format cookie file for
	// authenticated cloning. Cookies are loaded at startup and saved
	// on completion, enabling repeat cloning of login-protected sites.
	// Empty string = no cookie persistence.
	CookieFile string
}

// DefaultSkipAssetExts returns the default set of file extensions that should
// not be downloaded (media, archives, documents). These assets remain as live
// links in the cloned output.
func DefaultSkipAssetExts() map[string]bool {
	return map[string]bool{
		".mp4": true, ".m4v": true, ".webm": true, ".avi": true,
		".mov": true, ".mkv": true, ".wmv": true, ".flv": true,
		".mp3": true, ".wav": true, ".ogg": true, ".flac": true,
		".aac": true, ".m4a": true, ".wma": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true,
		".xlsx": true, ".ppt": true, ".pptx": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true,
		".7z": true, ".rar": true, ".dmg": true, ".exe": true,
		".msi": true, ".apk": true, ".iso": true,
	}
}

// DefaultEnhancedOptions returns sensible defaults.
func DefaultEnhancedOptions() EnhancedClonerOptions {
	return EnhancedClonerOptions{
		Workers:         4,
		AssetWorkers:    4,
		Timeout:         60 * time.Second,
		Settle:          1500 * time.Millisecond,
		Traversal:       TraversalBFS,
		RespectRobots:   true,
		EnableResume:    true,
		DedupContent:    true,
		MobileReadable:  true,
		AssetSameDomain: true,
		SkipAssetExts:       DefaultSkipAssetExts(),
		MaxAssetBytes:       50 * 1024 * 1024, // 50 MB.
		AntibotEnabled:      true,
		AntibotAutoEscalate: true,
		Incremental:         false,
		CacheMaxAge:         24 * time.Hour,
		UserAgent:           "Mozilla/5.0 (compatible; Wukong-Cloner/2.0)",
	}
}

// EnhancedCloner is the improved website cloning engine.
type EnhancedCloner struct {
	opts    EnhancedClonerOptions
	seedURL string
	host    string
	scheme  string

	// Browser pool for rendering pages.
	browserPool *browser.Pool

	// Frontier for URL deduplication and resume.
	front *frontier

	// Asset downloader for static resources.
	assetDownloader *AssetDownloader

	// Content deduper for saving disk space.
	deduper *ContentDeduper

	// Incremental cache for ETag/Last-Modified checks.
	cache *CloneCache

	// HTTP client for non-browser requests.
	httpClient *http.Client

	// robots.txt rules.
	robots *RobotsRule

	// Rate limiter for polite crawling.
	rateLimiter *RateLimiter

	// Anti-bot detection and auto-escalation engine.
	antibot *antibot.Engine

	// Page and asset job queues.
	pageJobs  chan pageJob
	assetJobs chan assetJob

	// DFS traversal support: page stack + dispatcher.
	pageStack      []pageJob    // DFS LIFO stack.
	pageMu         sync.Mutex
	pageReady      chan struct{} // Signal when new pages are available.
	dispatcherStop chan struct{} // Signal to stop the dispatcher.

	// Downloaded assets registry.
	downloadedAssets map[string]*downloadedAsset
	assetMu          sync.RWMutex

	// Stats and results.
	stats    Stats
	statsMu  sync.RWMutex
	results  []PageResult
	resultsMu sync.Mutex

	// Directories.
	pageDir  string
	assetDir string

	// Wait group for tracking active jobs.
	wg sync.WaitGroup

	// Track enqueued count for MaxPages limit.
	enqueuedPages int
	enqueuedMu    sync.Mutex
}

// pageJob represents a pending page rendering job.
type pageJob struct {
	url   string
	depth int
}

// assetJob represents a pending asset download job.
type assetJob struct {
	url string
}

// NewEnhancedCloner creates a new enhanced cloning engine.
func NewEnhancedCloner(opts EnhancedClonerOptions) *EnhancedCloner {
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.AssetWorkers <= 0 {
		opts.AssetWorkers = opts.Workers
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Mozilla/5.0 (compatible; Wukong-Cloner/2.0)"
	}

	dl := DefaultAssetDownloader()
	dl.UserAgent = opts.UserAgent
	if opts.MaxAssetBytes > 0 {
		dl.MaxBytes = opts.MaxAssetBytes
	}

	return &EnhancedCloner{
		opts:             opts,
		front:            newFrontier(),
		assetDownloader:  dl,
		deduper:          NewContentDeduper(!opts.DedupContent),
		downloadedAssets: make(map[string]*downloadedAsset),
		pageReady:        make(chan struct{}, 1),
		dispatcherStop:   make(chan struct{}),
	}
}

// Clone performs the website cloning operation.
func (ec *EnhancedCloner) Clone(ctx context.Context, seedURL string) (*Result, error) {
	startTime := time.Now()

	// Parse and normalize seed URL.
	parsedURL, err := url.Parse(seedURL)
	if err != nil {
		return nil, fmt.Errorf("parse seed URL: %w", err)
	}

	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
		seedURL = "https://" + seedURL
	}
	ec.scheme = parsedURL.Scheme
	ec.host = parsedURL.Host
	ec.seedURL = seedURL

	// Set up output directory.
	outputDir := ec.opts.OutputDir
	if outputDir == "" {
		homeDir, _ := os.UserHomeDir()
		outputDir = filepath.Join(homeDir, ".wukong_apps", "cloned", ec.host)
	}
	ec.opts.OutputDir = outputDir

	ec.pageDir = filepath.Join(outputDir, "pages")
	ec.assetDir = filepath.Join(outputDir, "assets")

	// Force clean if requested.
	if ec.opts.Force {
		os.RemoveAll(outputDir)
	}

	// Initialize incremental cache.
	if ec.opts.Incremental {
		cache, err := NewCloneCache("", ec.host)
		if err != nil {
			// Non-fatal: continue without cache.
			ec.cache = nil
		} else {
			cache.SetSeedURL(seedURL)
			if ec.opts.Force {
				cache.Clear()
			}
			ec.cache = cache
		}
	}

	// Create directories.
	for _, d := range []string{ec.pageDir, ec.assetDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Initialize cookie session for authenticated cloning.
	var cloneSess *CloneSession
	if ec.opts.CookieFile != "" {
		sess, err := NewCloneSession(ec.opts.CookieFile)
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"[wukong/session] cookie load failed: %v\n", err)
		} else {
			cloneSess = sess
			fmt.Fprintf(os.Stderr,
				"[wukong/session] cookies loaded from %s\n",
				ec.opts.CookieFile)
		}
	}

	// Initialize HTTP client (with cookie jar if session is active).
	if cloneSess != nil {
		ec.httpClient = cloneSess.HTTPClient()
	} else {
		ec.httpClient = &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(req *http.Request,
				via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		}
	}

	// Load robots.txt.
	if ec.opts.RespectRobots {
		robots, err := FetchRobots(ctx, ec.httpClient, ec.host, ec.scheme)
		if err != nil {
			// Non-fatal: continue without robots.txt.
		} else {
			ec.robots = robots
			// Set up rate limiting from crawl-delay.
			if ec.opts.CrawlDelay > 0 {
				ec.rateLimiter = NewRateLimiter(ec.opts.CrawlDelay)
			} else {
				ec.rateLimiter = NewRateLimiterFromCrawlDelay(robots.CrawlDelayDuration())
			}
		}
	} else {
		ec.rateLimiter = NewRateLimiter(100 * time.Millisecond)
	}

	// Resume from previous state.
	if ec.opts.EnableResume && !ec.opts.Refresh {
		statePath := frontierStatePath(outputDir)
		if err := ec.front.load(statePath); err != nil {
			// Non-fatal.
		}
	}

	// Create browser pool.
	pool := browser.NewPool(browser.PoolOptions{
		Headless:      true,
		Workers:       ec.opts.Workers,
		RenderTimeout: ec.opts.Timeout,
		Settle:        ec.opts.Settle,
		Scroll:        ec.opts.Scroll,
		Stealth:       ec.opts.Stealth,
		ChromePath:    ec.opts.ChromePath,
	})
	ec.browserPool = pool
	defer pool.Close()

	// Initialize anti-bot detection and auto-escalation engine.
	abCfg := antibot.Config{
		Enabled:      ec.opts.AntibotEnabled,
		AutoEscalate: ec.opts.AntibotAutoEscalate,
	}
	if ec.opts.Stealth {
		// If stealth is already enabled, start from LevelStealth
		// to raise the baseline immediately.
		abCfg.InitialLevel = antibot.LevelStealth
	}
	ec.antibot = antibot.New(abCfg)

	// Create job channels.
	// Buffered channels prevent deadlock when workers discover new pages
	// and try to enqueue them while all workers are busy processing.
	ec.pageJobs = make(chan pageJob, ec.opts.Workers*8)
	ec.assetJobs = make(chan assetJob, ec.opts.AssetWorkers*16)

	// Start traversal dispatcher: feeds pages to workers according to the
	// selected traversal strategy (FIFO for BFS, LIFO for DFS).
	go ec.traversalDispatcher(ctx)

	// Start page workers.
	for i := 0; i < ec.opts.Workers; i++ {
		go ec.pageWorker(ctx, i)
	}

	// Start asset workers.
	for i := 0; i < ec.opts.AssetWorkers; i++ {
		go ec.assetWorker(ctx, i)
	}

	// Enqueue seed URL.
	ec.enqueuePage(seedURL, 0)

	// Discover sitemaps for additional seeds.
	if !ec.opts.NoSitemap && ec.robots != nil && len(ec.robots.Sitemaps) > 0 {
		sitemapURLs, err := FetchSitemaps(ctx, ec.httpClient, ec.robots.Sitemaps)
		if err == nil {
			for _, su := range sitemapURLs {
				// Parse and validate.
				parsed, err := url.Parse(su)
				if err != nil {
					continue
				}
				// Only enqueue same-site URLs.
				if parsed.Host != ec.host && !ec.opts.Subdomains {
					continue
				}
				ec.enqueuePage(su, 1)
			}
		}
	}

	// Wait for all jobs to complete.
	ec.wg.Wait()

	// Signal the DFS dispatcher to stop (avoids goroutine leak).
	close(ec.dispatcherStop)

	// Close channels and wait for workers to drain.
	close(ec.pageJobs)
	close(ec.assetJobs)

	// Save incremental cache.
	if ec.cache != nil {
		ec.cache.UpdateLastSync()
		if err := ec.cache.Save(); err != nil {
			// Non-fatal.
		}
	}

	// Save frontier state.
	if ec.opts.EnableResume {
		statePath := frontierStatePath(outputDir)
		if err := ec.front.save(statePath); err != nil {
			// Non-fatal.
		}
	}

	// Save session cookies for authenticated cloning.
	if cloneSess != nil {
		if err := cloneSess.Save(); err != nil {
			fmt.Fprintf(os.Stderr,
				"[wukong/session] cookie save failed: %v\n", err)
		}
	}

	endTime := time.Now()

	// Build result.
	ec.statsMu.RLock()
	ec.resultsMu.Lock()
	dedupFiles, dedupSaved := ec.deduper.Savings()
	result := &Result{
		Success:            ec.stats.PagesFailed == 0,
		SeedURL:            ec.seedURL,
		Host:               ec.host,
		OutputDir:          outputDir,
		Pages:              ec.stats.PagesCloned,
		Assets:             ec.stats.AssetsDownloaded,
		SizeBytes:          ec.stats.TotalBytes,
		Duration:           endTime.Sub(startTime),
		DedupFiles:         dedupFiles,
		DedupBytesSaved:    dedupSaved,
		AntibotDetections:  len(ec.antibot.Escalator.History),
		AntibotStats:       ec.antibot.Stats(),
		StartTime:          startTime,
		EndTime:            endTime,
	}

	for _, pr := range ec.results {
		if pr.Error != "" {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: %s", pr.URL, pr.Error))
		}
	}
	ec.resultsMu.Unlock()
	ec.statsMu.RUnlock()

	return result, nil
}

// ---------------------------------------------------------------------------
// Worker goroutines.
// ---------------------------------------------------------------------------

// pageWorker processes pages from the page job channel.
func (ec *EnhancedCloner) pageWorker(ctx context.Context, id int) {
	for job := range ec.pageJobs {
		// Check for cancellation BEFORE processing.
		select {
		case <-ctx.Done():
			ec.wg.Done()
			return
		default:
		}

		result := ec.processPage(ctx, job.url, job.depth)

		// Update stats.
		ec.statsMu.Lock()
		if result.Error == "" {
			ec.stats.PagesCloned++
			ec.stats.TotalBytes += result.Size
		} else {
			ec.stats.PagesFailed++
		}
		ec.statsMu.Unlock()

		ec.resultsMu.Lock()
		ec.results = append(ec.results, result)
		ec.resultsMu.Unlock()

		// Done AFTER processing, so wg.Wait() correctly waits for completion.
		ec.wg.Done()
	}
}

// assetWorker processes asset downloads from the asset job channel.
func (ec *EnhancedCloner) assetWorker(ctx context.Context, id int) {
	for job := range ec.assetJobs {
		select {
		case <-ctx.Done():
			ec.wg.Done()
			return
		default:
		}

		if err := ec.processAsset(ctx, job.url); err != nil {
			ec.statsMu.Lock()
			ec.stats.PagesFailed++
			ec.statsMu.Unlock()
		}

		ec.wg.Done()
	}
}

// ---------------------------------------------------------------------------
// Page processing.
// ---------------------------------------------------------------------------

// processPage renders and saves a single page.
// Uses a single-pass DOM walk (sink callback) to simultaneously rewrite links
// and discover new pages/assets — eliminating the separate extract+rewrite
// two-pass approach.
func (ec *EnhancedCloner) processPage(ctx context.Context, pageURL string, depth int) PageResult {
	result := PageResult{
		URL:   pageURL,
		Depth: depth,
	}

	// Incremental cache check: skip rendering if page is unchanged.
	if ec.cache != nil && !ec.opts.Refresh {
		needsUpdate, reason, err := ec.cache.CheckNeedsUpdate(pageURL)
		if err == nil && !needsUpdate {
			// Page unchanged: reuse cached content.
			return ec.useCachedPage(pageURL, reason)
		}
		_ = err // If HEAD request fails, fall through to full render.
	}

	// Robots.txt check.
	if ec.robots != nil {
		parsed, err := url.Parse(pageURL)
		if err == nil {
			if !ec.robots.IsAllowed(parsed.Path) {
				result.Error = "blocked by robots.txt"
				return result
			}
		}
	}

	// Rate limiting.
	if ec.rateLimiter != nil {
		if err := ec.rateLimiter.Wait(ctx); err != nil {
			result.Error = fmt.Sprintf("rate limit: %v", err)
			return result
		}
	}

	// Anti-bot jitter delay (randomised pause for aggressive levels).
	ec.antibot.Wait()

	// Render page in headless Chrome.
	renderResult, err := ec.browserPool.Render(ctx, pageURL)
	if err != nil {
		// Check if the error indicates anti-bot blocking.
		if reason, _ := ec.antibot.CheckError(err); reason != antibot.ReasonNone {
			retry, delay, _, msg := ec.antibot.Escalate(
				pageURL, reason, 0)
			fmt.Fprintf(os.Stderr, "[wukong/antibot] %s\n", msg)
			ec.applyAntiBotLevel()
			if retry {
				select {
				case <-time.After(delay):
					// Re-enqueue for retry with escalated level.
					ec.enqueuePage(pageURL, depth)
					return result
				case <-ctx.Done():
					result.Error = "cancelled during anti-bot backoff"
					return result
				}
			}
		}
		result.Error = fmt.Sprintf("render: %v", err)
		return result
	}

	// Detect anti-bot patterns in the rendered page.
	abReason, abDesc := ec.antibot.CheckResponse(
		200, nil, renderResult.HTML)
	if abReason != antibot.ReasonNone {
		fmt.Fprintf(os.Stderr, "[wukong/antibot] %s at %s\n", abDesc, pageURL)
		retry, delay, _, msg := ec.antibot.Escalate(
			pageURL, abReason, 200)
		ec.applyAntiBotLevel()
		if retry {
			fmt.Fprintf(os.Stderr, "[wukong/antibot] %s\n", msg)
			select {
			case <-time.After(delay):
				ec.enqueuePage(pageURL, depth)
				return result
			case <-ctx.Done():
				result.Error = "cancelled during anti-bot backoff"
				return result
			}
		}
		// If not retrying, continue but record the detection.
		result.Error = "anti-bot page detected: " + abDesc
		return result
	}

	// Clean HTML with enhanced sanitization.
	cleanOpts := sanitize.CleanOptions{
		KeepNoscript:    false,
		KeepMetaRefresh: false,
		MobileReadable:  ec.opts.MobileReadable,
		Banner:          fmt.Sprintf("Cloned by Wukong from %s on %s",
			pageURL, time.Now().Format(time.RFC3339)),
	}
	cleanHTML, _ := sanitize.CleanHTMLWithOptions(renderResult.HTML, cleanOpts)

	// Determine local file path using deterministic mapping.
	localRelPath := PageKey(ec.host, pageURL)
	fullPath := filepath.Join(ec.pageDir, localRelPath)

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		result.Error = fmt.Sprintf("mkdir: %v", err)
		return result
	}

	// Single-pass DOM walk: rewrite links AND discover pages/assets.
	// This merges the former extractLinks + extractAssets + rewritePageLinks
	// into one traversal, using a sink callback.
	pageMirrorPath := filepath.ToSlash(filepath.Join("pages", localRelPath))
	rewrittenHTML := ec.rewriteAndDiscover(cleanHTML, pageURL, pageMirrorPath,
		depth, &result.LinksFound, &result.AssetsFound)

	contentBytes := []byte(rewrittenHTML)

	// Content dedup: if identical content exists, create hard link.
	deduped, err := ec.deduper.TryDedup(contentBytes, fullPath)
	if err != nil {
		// Fallback: write directly on dedup error.
		if writeErr := os.WriteFile(fullPath, contentBytes, 0644); writeErr != nil {
			result.Error = fmt.Sprintf("write: %v", writeErr)
			return result
		}
		ec.deduper.MarkWritten(contentBytes, fullPath)
	} else if deduped {
		// Successfully hard-linked to existing content.
		result.FilePath = fullPath
		result.Title = renderResult.Title
		result.Size = int64(len(contentBytes))
		return result
	} else {
		// New content, write to disk.
		if writeErr := os.WriteFile(fullPath, contentBytes, 0644); writeErr != nil {
			result.Error = fmt.Sprintf("write: %v", writeErr)
			return result
		}
		ec.deduper.MarkWritten(contentBytes, fullPath)
	}

	result.FilePath = fullPath
	result.Title = renderResult.Title
	result.Size = int64(len(rewrittenHTML))

	// Save to incremental cache for future runs.
	ec.updateCacheEntry(pageURL, fullPath, contentBytes)

	// Mark as visited.
	ec.front.markVisited(localRelPath)

	return result
}

// ---------------------------------------------------------------------------
// Asset processing.
// ---------------------------------------------------------------------------

// processAsset downloads and saves a single asset, rewriting CSS references.
func (ec *EnhancedCloner) processAsset(ctx context.Context, assetURL string) error {
	key := AssetKey(assetURL)

	// Check if already downloaded.
	ec.assetMu.RLock()
	if _, exists := ec.downloadedAssets[assetURL]; exists {
		ec.assetMu.RUnlock()
		return nil
	}
	ec.assetMu.RUnlock()

	// Download using the standalone asset downloader (with retries).
	assetResult, err := ec.assetDownloader.Download(ctx, assetURL)
	if err != nil {
		// Anti-bot check: was this asset blocked by HTTP status?
		var de *DownloadError
		if AsDownloadError(err, &de) && de.StatusCode > 0 {
			reason, desc := antibot.DetectHTTP(de.StatusCode, nil)
			if reason != antibot.ReasonNone {
				retry, delay, _, msg := ec.antibot.Escalate(
					assetURL, reason, de.StatusCode)
				ec.applyAntiBotLevel()
				fmt.Fprintf(os.Stderr,
					"[wukong/antibot] asset %s: %s. %s\n",
					assetURL, desc, msg)
				if retry {
					time.Sleep(delay)
					ec.wg.Add(1)
					ec.assetJobs <- assetJob{url: assetURL}
					return nil
				}
			}
		}
		ec.front.markVisited(key)
		return err
	}

	// Determine local path.
	localPath := LocalPath(ec.host, assetURL, KindAsset)
	fullPath := filepath.Join(ec.assetDir, localPath)

	// Ensure directory.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		ec.front.markVisited(key)
		return err
	}

	data := assetResult.Body
	contentType := assetResult.ContentType

	// Rewrite CSS url() references to local paths and discover new assets.
	if assetResult.IsCSS {
		var discovered []string

		// CSS file's mirror path for relative URL computation.
		cssMirrorPath := filepath.ToSlash(filepath.Join("assets", localPath))

		// Rewrite URL references using the CSS rewriter with relative paths.
		data = RewriteCSS(data, assetURL, func(absRef string) string {
			refKey := AssetKey(absRef)
			targetLocalPath := LocalPath(ec.host, absRef, KindAsset)
			targetMirrorPath := filepath.ToSlash(filepath.Join("assets", targetLocalPath))

			// Enqueue newly discovered assets for download.
			if ec.front.offer(refKey) {
				discovered = append(discovered, absRef)
			}

			// Return relative path from CSS file to the referenced asset.
			return Rel(cssMirrorPath, targetMirrorPath)
		})

		// Also extract all references for discovery (even non-rewritten ones).
		allRefs := ExtractCSSAssetRefs(assetResult.Body, assetURL)
		for _, refURL := range allRefs {
			refKey := AssetKey(refURL)
			if ec.front.offer(refKey) {
				discovered = append(discovered, refURL)
			}
		}

		// Enqueue all discovered CSS assets.
		for _, discURL := range discovered {
			ec.wg.Add(1)
			ec.assetJobs <- assetJob{url: discURL}
		}
	}

	// Write to disk.
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		ec.front.markVisited(key)
		return err
	}

	// Register downloaded asset.
	ec.assetMu.Lock()
	ec.downloadedAssets[assetURL] = &downloadedAsset{
		URL:         assetURL,
		LocalPath:   localPath,
		ContentType: contentType,
		Size:        int64(len(data)),
		MimeType:    contentType,
	}
	ec.assetMu.Unlock()

	ec.statsMu.Lock()
	ec.stats.AssetsDownloaded++
	ec.stats.TotalBytes += int64(len(data))
	ec.statsMu.Unlock()

	ec.front.markVisited(key)
	return nil
}

// ---------------------------------------------------------------------------
// Link rewriting (single-pass DOM walk with discovery).
// ---------------------------------------------------------------------------

// rewriteAndDiscover performs a single-pass DOM walk that simultaneously:
//   1. Rewrites all resource URLs to local relative paths.
//   2. Discovers new pages (enqueues them if in scope and under limits).
//   3. Discovers new assets (enqueues them if the policy allows download).
//
// This merges the former three-pass approach (extractLinks + extractAssets
// + rewritePageLinks) into one efficient traversal.
func (ec *EnhancedCloner) rewriteAndDiscover(htmlStr, pageURL, pageMirrorPath string,
	depth int, linksFound, assetsFound *int) string {

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return htmlStr
	}

	shouldCrawl := ec.shouldCrawlMore(depth)

	// Build the rewrite-and-discover sink: for each URL encountered,
	// compute its local path, enqueue it for crawling/download, and
	// return the relative path for the rewritten HTML.
	sink := func(absURL string, kind URLKind) string {
		var targetPath string
		switch kind {
		case KindPage:
			*linksFound++
			// Check if this page is in scope and should be crawled.
			if shouldCrawl {
				if u, err := url.Parse(absURL); err == nil {
					scopeCfg := ScopeConfig{
						AllowSubdomains: ec.opts.Subdomains,
						ScopePrefix:     ec.opts.ScopePrefix,
						ExcludePrefixes: ec.opts.Exclude,
					}
					seed, _ := url.Parse(ec.seedURL)
					if seed != nil && InScope(seed, u, scopeCfg) {
						ec.enqueuePage(absURL, depth+1)
					}
				}
			}
			targetPath = filepath.ToSlash(filepath.Join("pages",
				LocalPath(ec.host, absURL, KindPage)))

		case KindAsset:
			*assetsFound++
			// Only enqueue assets that the policy allows downloading.
			if ec.wantAsset(absURL) {
				key := AssetKey(absURL)
				if ec.front.offer(key) {
					ec.wg.Add(1)
					ec.assetJobs <- assetJob{url: absURL}
				}
			}
			targetPath = filepath.ToSlash(filepath.Join("assets",
				LocalPath(ec.host, absURL, KindAsset)))

		default:
			return "" // Keep original for unknown kinds.
		}

		return Rel(pageMirrorPath, targetPath)
	}

	RewriteHTML(doc, base, sink)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return htmlStr
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// Traversal dispatcher.
// ---------------------------------------------------------------------------

// traversalDispatcher feeds page jobs to workers according to the selected
// traversal strategy. For BFS, it reads from the pageJobs channel (FIFO).
// For DFS, it manages a LIFO stack and pushes items to pageJobs as workers
// become available.
func (ec *EnhancedCloner) traversalDispatcher(ctx context.Context) {
	if ec.opts.Traversal != TraversalDFS {
		return // BFS: pages go directly to pageJobs channel.
	}

	for {
		// Wait for pages, stop signal, or context cancellation.
		select {
		case <-ctx.Done():
			return
		case <-ec.dispatcherStop:
			return
		case <-ec.pageReady:
			// Drain the stack into the pageJobs channel (LIFO → reverse).
			ec.drainStack(ctx)
		}
	}
}

// drainStack pops all items from the page stack and sends them to the
// pageJobs channel in LIFO order (deepest pages first for DFS).
func (ec *EnhancedCloner) drainStack(ctx context.Context) {
	for {
		ec.pageMu.Lock()
		if len(ec.pageStack) == 0 {
			ec.pageMu.Unlock()
			return
		}
		// Pop from end (LIFO).
		job := ec.pageStack[len(ec.pageStack)-1]
		ec.pageStack = ec.pageStack[:len(ec.pageStack)-1]
		ec.pageMu.Unlock()

		select {
		case ec.pageJobs <- job:
		case <-ctx.Done():
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// enqueuePage adds a page URL to the crawl queue.
func (ec *EnhancedCloner) enqueuePage(pageURL string, depth int) {
	// Validate URL.
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return
	}

	// Only HTTP(S).
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return
	}

	// Scope check: same host or subdomain.
	scopeCfg := ScopeConfig{
		AllowSubdomains: ec.opts.Subdomains,
		ScopePrefix:     ec.opts.ScopePrefix,
		ExcludePrefixes: ec.opts.Exclude,
	}

	seed, _ := url.Parse(ec.seedURL)
	if seed != nil && !InScope(seed, parsed, scopeCfg) {
		return
	}

	// Normalize and get key.
	canonURL, err := Normalize(ec.seedURL, pageURL)
	if err != nil {
		return
	}
	key := PageKey(ec.host, canonURL)

	// Check MaxPages limit.
	if ec.opts.MaxPages > 0 {
		ec.enqueuedMu.Lock()
		if ec.enqueuedPages >= ec.opts.MaxPages {
			ec.enqueuedMu.Unlock()
			return
		}
		ec.enqueuedMu.Unlock()
	}

	// Offer to frontier for dedup.
	if !ec.front.offer(key) {
		return // Already seen.
	}

	// Check MaxDepth.
	if ec.opts.MaxDepth > 0 && depth > ec.opts.MaxDepth {
		return
	}

	ec.enqueuedMu.Lock()
	ec.enqueuedPages++
	ec.enqueuedMu.Unlock()

	ec.wg.Add(1)

	// BFS vs DFS: enqueue to channel (FIFO) or push to stack (LIFO).
	if ec.opts.Traversal == TraversalDFS {
		ec.pageMu.Lock()
		ec.pageStack = append(ec.pageStack, pageJob{url: canonURL, depth: depth})
		ec.pageMu.Unlock()
		// Signal dispatcher that new pages are available.
		select {
		case ec.pageReady <- struct{}{}:
		default:
		}
	} else {
		ec.pageJobs <- pageJob{url: canonURL, depth: depth}
	}
}

// shouldCrawlMore checks whether more links should be followed.
func (ec *EnhancedCloner) shouldCrawlMore(depth int) bool {
	if ec.opts.MaxDepth > 0 && depth >= ec.opts.MaxDepth {
		return false
	}
	if ec.opts.MaxPages > 0 {
		ec.statsMu.RLock()
		pages := ec.stats.PagesCloned
		ec.statsMu.RUnlock()
		if pages >= ec.opts.MaxPages {
			return false
		}
	}
	return true
}

// applyAntiBotLevel applies the current antibot escalation level to the
// browser pool. When the antibot engine escalates to Level 2+ (Stealth),
// this dynamically injects anti-detection scripts into the running browser
// so all subsequent page loads are stealth-enabled without a restart.
func (ec *EnhancedCloner) applyAntiBotLevel() {
	if ec.antibot == nil || ec.browserPool == nil {
		return
	}

	level := ec.antibot.Level()

	// If the antibot engine needs stealth script but it's not yet active,
	// dynamically enable it in the browser pool.
	if ec.antibot.NeedsStealthScript() && !ec.browserPool.StealthEnabled() {
		if err := ec.browserPool.EnableStealth(); err != nil {
			fmt.Fprintf(os.Stderr,
				"[wukong/antibot] failed to enable stealth: %v\n", err)
		}
	}

	// Aggressive level: rotate User-Agent to diversify fingerprint.
	if level >= antibot.LevelAggressive {
		rotatedUA := ec.antibot.Escalator.RotateUserAgent()
		ec.assetDownloader.UserAgent = rotatedUA
		fmt.Fprintf(os.Stderr,
			"[wukong/antibot] UA rotated for aggressive mode\n")
	}
}

// wantAsset reports whether an asset should be downloaded and localised.
// Two filtering policies:
//   1. AssetSameDomain: skip assets on hosts outside the seed's registrable
//      domain (CDNs, analytics, third-party trackers).
//   2. SkipAssetExts: skip assets whose file extension is in the skip set
//      (media files, archives, documents). These remain as live links.
func (ec *EnhancedCloner) wantAsset(assetURL string) bool {
	u, err := url.Parse(assetURL)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// AssetSameDomain: only download same-registrable-domain assets.
	if ec.opts.AssetSameDomain {
		seed, _ := url.Parse(ec.seedURL)
		if seed != nil && !SameRegistrableDomain(seed, u) {
			return false
		}
	}

	// SkipAssetExts: skip bulk media, documents, archives, installers.
	ext := strings.ToLower(PathExt(assetURL))
	if ec.opts.SkipAssetExts[ext] {
		return false
	}

	return true
}

// useCachedPage returns a cached page result when the page hasn't changed.
func (ec *EnhancedCloner) useCachedPage(pageURL string, reason string) PageResult {
	result := PageResult{
		URL:       pageURL,
		FromCache: true,
	}

	entry := ec.cache.GetEntry(pageURL)
	if entry == nil || entry.LocalPath == "" {
		result.Error = "cache miss (entry not found)"
		return result
	}

	// Verify the cached file still exists on disk.
	if _, err := os.Stat(entry.LocalPath); err != nil {
		result.Error = fmt.Sprintf("cached file gone: %v", err)
		return result
	}

	result.FilePath = entry.LocalPath
	result.Size = entry.Size
	result.Title = "(cached)"

	// Mark as visited in frontier so resume works correctly.
	key := PageKey(ec.host, pageURL)
	ec.front.markVisited(key)

	return result
}

// updateCacheEntry stores a rendered page in the incremental cache.
func (ec *EnhancedCloner) updateCacheEntry(pageURL, localPath string, content []byte) {
	if ec.cache == nil {
		return
	}
	entry := &CacheEntry{
		URL:         pageURL,
		LocalPath:   localPath,
		ContentHash: sha256Hex(content),
		LastFetched: time.Now(),
		StatusCode:  200,
		Size:        int64(len(content)),
	}
	ec.cache.SetEntry(entry)
}


