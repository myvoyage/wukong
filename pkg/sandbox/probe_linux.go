//go:build linux

package sandbox

// Stub probe functions for non-Linux platforms.
// sandbox_darwin.go and sandbox_windows.go are excluded on Linux.

func probeDarwin() ProbeResult {
	return ProbeResult{
		Platform: "darwin",
		Backend:  "none",
		Warning:  "not running on macOS",
	}
}

func probeWindows() ProbeResult {
	return ProbeResult{
		Platform: "windows",
		Backend:  "none",
		Warning:  "not running on Windows",
	}
}
