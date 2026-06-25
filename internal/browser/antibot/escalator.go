package antibot

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Level defines the stealth escalation level.
type Level int

const (
	// LevelNone — no anti-bot measures, default behaviour.
	LevelNone Level = 0

	// LevelFlags — Chrome anti-detection flags only (no JS injection).
	// Adds --disable-blink-features=AutomationControlled and related flags.
	LevelFlags Level = 1

	// LevelStealth — full stealth JS injection + all Chrome flags.
	// Injects anti-detection scripts via Page.addScriptToEvaluateOnNewDocument.
	LevelStealth Level = 2

	// LevelAggressive — LevelStealth + randomised delays + UA rotation.
	// Adds 2-8s random delays between requests and rotates User-Agent.
	LevelAggressive Level = 3

	// LevelBackoff — exponential backoff + report failure.
	// Gives up on the current URL after retries, logs the incident.
	LevelBackoff Level = 4
)

// String returns a human-readable level name.
func (l Level) String() string {
	switch l {
	case LevelNone:
		return "none"
	case LevelFlags:
		return "flags"
	case LevelStealth:
		return "stealth"
	case LevelAggressive:
		return "aggressive"
	case LevelBackoff:
		return "backoff"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// EscalationEvent records a single escalation decision.
type EscalationEvent struct {
	Time     time.Time
	URL      string
	Reason   BlockReason
	OldLevel Level
	NewLevel Level
	Retry    bool
}

// Escalator manages the auto-escalation of anti-bot measures
// based on detected blocking patterns.
type Escalator struct {
	mu sync.Mutex

	// CurrentLevel is the active stealth level.
	CurrentLevel Level

	// MaxLevel caps the maximum escalation (default LevelAggressive).
	MaxLevel Level

	// AutoEscalate enables automatic escalation on detection.
	AutoEscalate bool

	// Per-URL retry counts for LevelBackoff.
	retries map[string]int

	// MaxRetries per URL before giving up.
	MaxRetries int

	// Cooldown between escalation attempts for the same URL.
	Cooldown time.Duration

	// History of escalation events for diagnostics.
	History []EscalationEvent

	// Random source for jitter.
	rng *rand.Rand
}

// EscalatorConfig configures the auto-escalation engine.
type EscalatorConfig struct {
	// InitialLevel is the starting stealth level (default LevelNone).
	InitialLevel Level

	// MaxLevel caps escalation (default LevelAggressive).
	MaxLevel Level

	// AutoEscalate enables automatic escalation (default true).
	AutoEscalate bool

	// MaxRetries per URL before giving up (default 3).
	MaxRetries int

	// Cooldown between retries of the same URL (default 30s).
	Cooldown time.Duration
}

// DefaultEscalatorConfig returns sensible defaults.
func DefaultEscalatorConfig() EscalatorConfig {
	return EscalatorConfig{
		InitialLevel: LevelNone,
		MaxLevel:     LevelAggressive,
		AutoEscalate: true,
		MaxRetries:   3,
		Cooldown:     30 * time.Second,
	}
}

// NewEscalator creates a new auto-escalation engine.
func NewEscalator(cfg EscalatorConfig) *Escalator {
	return &Escalator{
		CurrentLevel: cfg.InitialLevel,
		MaxLevel:     cfg.MaxLevel,
		AutoEscalate: cfg.AutoEscalate,
		retries:      make(map[string]int),
		MaxRetries:   cfg.MaxRetries,
		Cooldown:     cfg.Cooldown,
		History:      make([]EscalationEvent, 0, 64),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Check evaluates whether anti-bot blocking was detected and returns
// the recommended action. It auto-escalates if configured.
//
// Returns:
//   - shouldRetry: whether the caller should retry the URL
//   - retryDelay: how long to wait before retrying
//   - newLevel: the new stealth level to use (caller should apply if
//     different from current)
//   - message: human-readable diagnostic message
func (e *Escalator) Check(url string, reason BlockReason,
	statusCode int) (shouldRetry bool, retryDelay time.Duration,
	newLevel Level, message string) {

	e.mu.Lock()
	defer e.mu.Unlock()

	oldLevel := e.CurrentLevel

	// Record the hit.
	e.retries[url]++

	if !e.AutoEscalate || !ShouldRetry(reason) {
		return false, 0, e.CurrentLevel,
			fmt.Sprintf("non-retryable block: %s (HTTP %d)", reason, statusCode)
	}

	// Check if we've exceeded max retries for this URL.
	if e.retries[url] > e.MaxRetries {
		e.CurrentLevel = LevelBackoff
		e.recordEvent(url, reason, oldLevel, LevelBackoff, false)
		return false, 0, LevelBackoff,
			fmt.Sprintf("max retries (%d) exceeded for %s", e.MaxRetries, url)
	}

	// Escalate one level (unless already at max).
	newLevel = oldLevel + 1
	if newLevel > e.MaxLevel {
		newLevel = e.MaxLevel
	}
	e.CurrentLevel = newLevel

	// Calculate retry delay.
	delay := e.Cooldown
	switch newLevel {
	case LevelAggressive:
		// Add random jitter: 2x-4x base cooldown.
		delay = e.Cooldown +
			time.Duration(e.rng.Int63n(int64(e.Cooldown*3)))
	case LevelStealth:
		// Moderate jitter: 1x-2x cooldown.
		delay = e.Cooldown +
			time.Duration(e.rng.Int63n(int64(e.Cooldown)))
	default:
		// Minimal delay.
		delay = e.Cooldown / 2
	}

	e.recordEvent(url, reason, oldLevel, newLevel, true)

	return true, delay, newLevel,
		fmt.Sprintf("escalated %s → %s for %s (retry %d/%d, delay %v)",
			oldLevel, newLevel, url, e.retries[url], e.MaxRetries, delay)
}

// RetryCount returns the number of retries for a URL.
func (e *Escalator) RetryCount(url string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.retries[url]
}

// RotateUserAgent returns a random realistic browser User-Agent string.
// Used when escalation reaches LevelAggressive to diversify the
// fingerprint seen by anti-bot systems.
func (e *Escalator) RotateUserAgent() string {
	e.mu.Lock()
	idx := e.rng.Intn(len(userAgentPool))
	e.mu.Unlock()
	return userAgentPool[idx]
}

// userAgentPool contains a mix of Chrome, Firefox, and Safari UAs
// sampled from real-world traffic distributions.
var userAgentPool = []string{
	// Chrome 124 on macOS.
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	// Chrome 124 on Windows.
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	// Chrome 125 on macOS.
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	// Chrome 125 on Windows.
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	// Firefox 126 on Windows.
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) " +
		"Gecko/20100101 Firefox/126.0",
	// Firefox 126 on macOS.
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) " +
		"Gecko/20100101 Firefox/126.0",
	// Edge 124 on Windows.
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
	// Safari 17.4 on macOS.
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 " +
		"(KHTML, like Gecko) Version/17.4 Safari/605.1.15",
	// Chrome 124 on Linux.
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
}

// Reset clears escalation state (useful when restarting a clone).
func (e *Escalator) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.CurrentLevel = LevelNone
	e.retries = make(map[string]int)
	e.History = e.History[:0]
}

// JitterDelay returns a random delay for the current level.
func (e *Escalator) JitterDelay() time.Duration {
	e.mu.Lock()
	level := e.CurrentLevel
	e.mu.Unlock()

	switch level {
	case LevelAggressive:
		return time.Duration(2+e.rng.Int63n(6)) * time.Second // 2-8s
	case LevelStealth:
		return time.Duration(1 + e.rng.Int63n(3)) * time.Second // 1-4s
	case LevelFlags:
		return time.Duration(500+e.rng.Int63n(1500)) * time.Millisecond // 0.5-2s
	default:
		return 0
	}
}

// Stats returns a diagnostic summary of the escalation engine.
func (e *Escalator) Stats() string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.History) == 0 {
		return "antibot: no blocking detected"
	}

	var reasons []string
	for _, ev := range e.History {
		reasons = append(reasons, string(ev.Reason))
	}
	reasonCounts := make(map[string]int)
	for _, r := range reasons {
		reasonCounts[r]++
	}

	return fmt.Sprintf(
		"antibot: level=%s, events=%d, blocked=%d URLs",
		e.CurrentLevel, len(e.History), len(e.retries),
	)
}

func (e *Escalator) recordEvent(url string, reason BlockReason,
	oldLevel, newLevel Level, retry bool) {
	e.History = append(e.History, EscalationEvent{
		Time:     time.Now(),
		URL:      url,
		Reason:   reason,
		OldLevel: oldLevel,
		NewLevel: newLevel,
		Retry:    retry,
	})
}

// Wait blocks for the jitter delay of the current level.
func (e *Escalator) Wait(ctx context.Context) error {
	delay := e.JitterDelay()
	if delay == 0 {
		return nil
	}
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
