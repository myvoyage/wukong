//go:build !linux && !darwin && !windows

package sandbox

import (
	"os/exec"
	"runtime"
)

// On unsupported platforms (FreeBSD, OpenBSD, etc.), sandboxing is not
// available. All commands run unsandboxed. Callers should check
// Available() or Probe() to determine if enforcement is active.

func available() bool { return false }

func reasonUnavailable() string {
	return "sandboxing not supported on " + runtime.GOOS
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	return nil
}
