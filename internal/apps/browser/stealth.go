// Package browser provides headless Chrome browser pool management.
//
// stealth.go: Anti-detection (stealth mode) wrapper around the shared
// internal/browser/stealth module. Injects a pre-load script that hides
// automation indicators from common bot-detection libraries.
package browser

import (
	"github.com/chromedp/chromedp"
	"github.com/km269/wukong/internal/browser/stealth"
)

// injectStealthAction returns a chromedp.Action that injects the shared
// anti-detection script via CDP Page.addScriptToEvaluateOnNewDocument.
func injectStealthAction() chromedp.Action {
	return stealth.InjectAction()
}
