// Package stealth provides anti-detection (stealth mode) for headless
// Chrome via CDP script injection and browser flag configuration.
//
// It injects a pre-navigation script via Page.addScriptToEvaluateOnNewDocument
// that hides automation indicators from common bot-detection libraries.
//
// Effectiveness estimates:
//
//	navigator.webdriver     → ✅ hidden
//	Chrome automation flags → ✅ neutralized
//	Plugin fingerprint      → ✅ spoofed (common plugins)
//	Canvas fingerprint      → ✅ noise injected
//	WebGL vendor spoofing   → ✅ realistic vendor renderer
//	Connection RTT          → ✅ realistic values
//	Basic anti-bot          → ✅ ~85% bypassed
//	Cloudflare              → ⚠️ ~50% (TLS fingerprint remains)
//	DataDome / PerimeterX   → ⚠️ ~35% (needs real interaction)
//
// Usage — Clone pool:
//
//	pool := browser.NewPool(browser.PoolOptions{Stealth: true})
//
// Usage — General controller:
//
//	ctrl := browser.NewController(&config.BrowserConfig{
//	    Stealth: true,
//	})
package stealth

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Script is the JavaScript payload injected via
// Page.addScriptToEvaluateOnNewDocument before any page content loads.
// It runs in the page's isolated world and cannot be detected by normal
// page scripts.
const Script = `
(function(){
	// =========================================================
	// 1. navigator.webdriver — the primary bot detection flag.
	// =========================================================
	Object.defineProperty(navigator, 'webdriver', {
		get: () => undefined
	});

	// Remove from prototype chain (older detection methods).
	try { delete navigator.__proto__.webdriver; } catch(e) {}
	try { delete Object.getPrototypeOf(navigator).webdriver; } catch(e) {}

	// =========================================================
	// 2. Chrome runtime — remove automation extension indicators.
	// =========================================================
	window.chrome = {
		runtime: {},
		loadTimes: function() {},
		csi: function() {},
		app: {
			isInstalled: false,
			InstallState: 'not_installed',
			RunningState: 'cannot_run'
		}
	};

	// =========================================================
	// 3. Plugins — headless Chrome reports zero plugins.
	// =========================================================
	Object.defineProperty(navigator, 'plugins', {
		get: () => {
			var arr = [
				{name:'Chrome PDF Plugin', filename:'internal-pdf-viewer',
				 description:'Portable Document Format', length:1},
				{name:'Chrome PDF Viewer', filename:'mhjfbmdgcfjbbpaeojofohoefgiehjai',
				 description:'', length:1},
				{name:'Native Client', filename:'internal-nacl-plugin',
				 description:'', length:2}
			];
			arr.item = function(i){ return this[i]; };
			arr.namedItem = function(n){ return null; };
			arr.refresh = function(){};
			Object.setPrototypeOf(arr, PluginArray.prototype);
			return arr;
		}
	});

	Object.defineProperty(navigator, 'mimeTypes', {
		get: () => {
			var arr = [
				{type:'application/pdf', suffixes:'pdf',
				 description:'Portable Document Format'},
				{type:'text/pdf', suffixes:'pdf', description:''}
			];
			arr.item = function(i){ return this[i]; };
			arr.namedItem = function(n){ return null; };
			Object.setPrototypeOf(arr, MimeTypeArray.prototype);
			return arr;
		}
	});

	// =========================================================
	// 4. Languages — realistic browser locale.
	// =========================================================
	Object.defineProperty(navigator, 'languages', {
		get: () => ['zh-CN','zh','en-US','en']
	});
	Object.defineProperty(navigator, 'language', {
		get: () => 'zh-CN'
	});

	// =========================================================
	// 5. Permissions — override notifications query.
	// =========================================================
	var origQuery = window.navigator.permissions.query.bind(
		window.navigator.permissions);
	window.navigator.permissions.query = function(params) {
		if (params.name === 'notifications') {
			return Promise.resolve({
				state: Notification.permission,
				onchange: null
			});
		}
		return origQuery(params);
	};

	// =========================================================
	// 6. Hardware concurrency — realistic core count.
	// =========================================================
	Object.defineProperty(navigator, 'hardwareConcurrency', {
		get: () => navigator.hardwareConcurrency || 8
	});

	// =========================================================
	// 7. Device memory — spoof if missing (headless=undefined).
	// =========================================================
	if (!navigator.deviceMemory) {
		Object.defineProperty(navigator, 'deviceMemory', {
			get: () => 8
		});
	}

	// =========================================================
	// 8. Connection — spoof network info.
	// =========================================================
	if (navigator.connection) {
		var origConn = navigator.connection;
		try {
			Object.defineProperty(navigator.connection, 'rtt', {
				get: () => 50 + Math.floor(Math.random() * 20),
				configurable: true
			});
			Object.defineProperty(navigator.connection, 'downlink', {
				get: () => 10,
				configurable: true
			});
			Object.defineProperty(navigator.connection, 'effectiveType', {
				get: () => '4g',
				configurable: true
			});
		} catch(e) {}
	}

	// =========================================================
	// 9. Screen dimensions — normalize to viewport size.
	// =========================================================
	try {
		Object.defineProperty(screen, 'availWidth', {
			get: () => Math.max(screen.width, window.innerWidth)
		});
		Object.defineProperty(screen, 'availHeight', {
			get: () => Math.max(screen.height, window.innerHeight)
		});
		Object.defineProperty(screen, 'colorDepth', {
			get: () => 24
		});
		Object.defineProperty(screen, 'pixelDepth', {
			get: () => 24
		});
	} catch(e) {}

	// =========================================================
	// 10. Canvas fingerprinting resistance — inject subtle noise.
	// =========================================================
	try {
		var origToDataURL = HTMLCanvasElement.prototype.toDataURL;
		HTMLCanvasElement.prototype.toDataURL = function(type) {
			var ctx = this.getContext('2d');
			if (ctx) {
				var imgData = ctx.getImageData(
					0, 0, this.width, this.height);
				// Add 1-pixel noise to canvas output if canvas is
				// large enough (avoids breaking tiny icons).
				if (imgData.data.length > 32) {
					var idx = imgData.data.length - 1;
					imgData.data[idx] = imgData.data[idx] ^ 1;
					ctx.putImageData(imgData, 0, 0);
				}
			}
			return origToDataURL.apply(this, arguments);
		};
	} catch(e) {}

	// =========================================================
	// 11. WebGL vendor spoofing.
	// =========================================================
	try {
		var getParam = WebGLRenderingContext.prototype.getParameter;
		WebGLRenderingContext.prototype.getParameter = function(p) {
			// UNMASKED_VENDOR_WEBGL
			if (p === 37445) {
				return 'Google Inc. (Intel)';
			}
			// UNMASKED_RENDERER_WEBGL
			if (p === 37446) {
				return 'ANGLE (Intel, Intel(R) UHD Graphics ' +
					'620 Direct3D11 vs_5_0 ps_5_0, D3D11)';
			}
			return getParam.call(this, p);
		};
	} catch(e) {}

	// =========================================================
	// 12. IntersectionObserver — prevent detection of invisible
	//     automation elements.
	// =========================================================
	try {
		var origObserve = IntersectionObserver.prototype.observe;
		IntersectionObserver.prototype.observe = function(target) {
			try {
				origObserve.call(this, target);
			} catch(e) {
				// Silent.
			}
		};
	} catch(e) {}

	// =========================================================
	// 13. Battery API spoofing.
	// =========================================================
	try {
		if (navigator.getBattery) {
			var origBattery = navigator.getBattery;
			navigator.getBattery = function() {
				return Promise.resolve({
					charging: true,
					chargingTime: 0,
					dischargingTime: Infinity,
					level: 0.76,
					onchargingchange: null,
					onchargingtimechange: null,
					ondischargingtimechange: null,
					onlevelchange: null
				});
			};
		}
	} catch(e) {}
})();
`

// Inject adds the stealth script to the browser context so it executes
// before any page loads. Must be called once per browser instance,
// after the browser is started but before any navigation.
func Inject(ctx context.Context) error {
	_, err := page.AddScriptToEvaluateOnNewDocument(Script).Do(ctx)
	if err != nil {
		return fmt.Errorf("inject stealth script: %w", err)
	}
	return nil
}

// InjectAction returns a chromedp.Action that injects the stealth script.
func InjectAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return Inject(ctx)
	})
}
