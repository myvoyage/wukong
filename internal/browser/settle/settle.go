// Package settle provides a shared network-idle wait strategy for headless
// Chrome. It monitors network events (loading finished, data received,
// request/response) and returns only after no network activity for a
// configurable quiet period.
//
// Used by both the clone browser pool and the general browser controller,
// replacing the former fixed chromedp.Sleep() pattern.
package settle

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Wait blocks until network activity on tabCtx has been idle for at least
// quiet. It monitors four Chrome DevTools Protocol network events and
// resets an idle timer on each. If the total wait exceeds quiet+10s,
// Wait returns without error (the page may still be functional).
func Wait(tabCtx context.Context, quiet time.Duration) error {
	timeout := quiet + 10*time.Second
	timeoutCtx, cancel := context.WithTimeout(tabCtx, timeout)
	defer cancel()

	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	// Listen for network events that indicate ongoing page loading.
	chromedp.ListenTarget(tabCtx, func(v any) {
		switch v.(type) {
		case *network.EventLoadingFinished,
			*network.EventDataReceived,
			*network.EventRequestWillBeSent,
			*network.EventResponseReceived:
			lastActivity.Store(time.Now().UnixNano())
		}
	})

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return nil // Non-fatal: page may still be usable.
		case <-ticker.C:
			idleFor := time.Duration(time.Now().UnixNano() - lastActivity.Load())
			if idleFor >= quiet {
				return nil
			}
		}
	}
}
