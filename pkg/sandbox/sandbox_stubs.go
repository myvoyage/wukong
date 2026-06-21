//go:build !linux && !darwin && !windows

package sandbox

// Stub probe functions for platforms that don't support sandboxing.
// These ensure the package compiles on all platforms while providing
// clear diagnostics about lack of sandbox support.

func probeLinux() ProbeResult {
	return ProbeResult{
		Platform: "linux",
		Backend:  "none",
		Warning:  "not running on Linux",
	}
}

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
