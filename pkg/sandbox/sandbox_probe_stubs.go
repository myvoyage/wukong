//go:build windows

package sandbox

// Stub probe functions for platforms other than the current one.
// These are needed on Windows where sandbox_linux.go and
// sandbox_darwin.go are excluded by build tags.

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
