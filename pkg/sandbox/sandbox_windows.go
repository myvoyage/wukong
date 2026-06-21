//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows backend: Low Integrity Level + Restricted Token.
// Low IL processes can read but not write Medium/High IL objects.
// All APIs are built into Windows since Vista — no extra installs.

func available() bool { return true }

func reasonUnavailable() string {
	if runtime.GOOS != "windows" {
		return "not Windows"
	}
	return ""
}

func probeWindows() ProbeResult {
	return ProbeResult{
		Sandboxed: true,
		Platform:  "windows",
		Backend:   "integrity-level",
	}
}

var (
	lowIL    *windows.SID
	lowILErr error
	lowILOnce sync.Once
)

// getLowIL returns the Low Integrity SID, creating it lazily.
// Uses sync.Once to avoid init-time panic and ensure thread safety.
func getLowIL() (*windows.SID, error) {
	lowILOnce.Do(func() {
		lowIL, lowILErr = windows.StringToSid("S-1-16-4096")
	})
	return lowIL, lowILErr
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	if err := setLowLabelOnDirs(ctx.writable); err != nil {
		return fmt.Errorf("sandbox: label directories: %w", err)
	}

	var token windows.Token
	if err := windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|
			windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ASSIGN_PRIMARY,
		&token,
	); err != nil {
		return fmt.Errorf("sandbox: open token: %w", err)
	}
	defer token.Close()

	var dupToken windows.Token
	if err := windows.DuplicateTokenEx(
		token,
		windows.TOKEN_ALL_ACCESS,
		nil,
		windows.SecurityAnonymous,
		windows.TokenPrimary,
		&dupToken,
	); err != nil {
		return fmt.Errorf("sandbox: duplicate token: %w", err)
	}

	if err := setTokenLowIL(dupToken); err != nil {
		dupToken.Close()
		return err
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Token = syscall.Token(dupToken)
	return nil
}

func setTokenLowIL(token windows.Token) error {
	sid, err := getLowIL()
	if err != nil {
		return fmt.Errorf("sandbox: create Low IL SID: %w", err)
	}

	type sidAndAttrs struct {
		Sid        *windows.SID
		Attributes uint32
	}
	type mandatoryLabel struct {
		Label sidAndAttrs
	}
	info := mandatoryLabel{
		Label: sidAndAttrs{
			Sid:        sid,
			Attributes: 0x20, // SE_GROUP_INTEGRITY
		},
	}
	return windows.SetTokenInformation(
		token,
		windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
}

func setLowLabelOnDirs(dirs []string) error {
	for _, dir := range dirs {
		abs, err := windows.FullPath(dir)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", dir, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("stat %q: %w", abs, err)
		}
		if err := setLowLabel(abs); err != nil {
			return fmt.Errorf("label %q: %w", abs, err)
		}
	}
	return nil
}

func setLowLabel(path string) error {
	sid, err := getLowIL()
	if err != nil {
		return fmt.Errorf("sandbox: get Low IL SID: %w", err)
	}
	ea := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_READ | windows.GENERIC_WRITE | windows.GENERIC_EXECUTE,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.CONTAINER_INHERIT_ACE | windows.OBJECT_INHERIT_ACE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{ea}, nil)
	if err != nil {
		return fmt.Errorf("build acl: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.LABEL_SECURITY_INFORMATION,
		nil, nil, nil, acl,
	); err != nil {
		return fmt.Errorf("SetNamedSecurityInfo: %w", err)
	}
	return nil
}
