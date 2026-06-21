//go:build darwin

package sandbox

// Stub probe functions for non-macOS platforms.
// sandbox_linux.go and sandbox_windows.go are excluded on Darwin.

func probeLinux() ProbeResult {
	return ProbeResult{
		Platform: "linux",
		Backend:  "none",
		Warning:  "not running on Linux",
	}
}

func probeWindows() ProbeResult {
	return ProbeResult{
		Platform: "windows",
		Backend:  "none",
		Warning:  "not running on Windows",
	}
}
