//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

// Linux sandbox backend uses Landlock (kernel 5.13+), a kernel built-in
// LSM that requires no extra packages or daemons.
//
// Self-exec helper pattern:
//  1. applySandbox rewrites the Cmd to point at this binary with
//     __SANDBOX_HELPER=1 and a JSON config in env.
//  2. helper_linux.go init() detects helper mode, applies Landlock,
//     and syscall.Exec's the real command.
//  3. The real command runs with Landlock-enforced FS restrictions.
//
// selfExePath is cached at init() to avoid repeated os.Executable()
// calls (which read /proc/self/exe) for every sandboxed command.
var selfExePath string

func init() {
	selfExePath, _ = os.Executable()
}

const (
	landlockCreateRuleset   = 444
	landlockAddRule         = 445
	landlockRestrictSelf    = 446
	landlockRulePathBeneath = 1
	landlockCreateVersion   = 1

	landlockAccessExecute    = 1 << 0
	landlockAccessWriteFile  = 1 << 1
	landlockAccessReadFile   = 1 << 2
	landlockAccessReadDir    = 1 << 3
	landlockAccessRemoveDir  = 1 << 4
	landlockAccessRemoveFile = 1 << 5
	landlockAccessMakeChar   = 1 << 6
	landlockAccessMakeDir    = 1 << 7
	landlockAccessMakeReg    = 1 << 8
	landlockAccessMakeSock   = 1 << 9
	landlockAccessMakeSym    = 1 << 12

	// ABI >= 2
	landlockAccessRefer = 1 << 13
	// ABI >= 3
	landlockAccessTruncate = 1 << 14

	landlockAccessWriteABI1 = landlockAccessWriteFile |
		landlockAccessRemoveFile |
		landlockAccessRemoveDir |
		landlockAccessMakeChar |
		landlockAccessMakeDir |
		landlockAccessMakeReg |
		landlockAccessMakeSock |
		landlockAccessMakeSym

	landlockAccessAllABI1 = landlockAccessExecute |
		landlockAccessWriteABI1 |
		landlockAccessReadFile |
		landlockAccessReadDir

	atFDCWD = -100
)

type landlockRulesetAttr struct {
	HandledAccessFS uint64
}

type landlockPathBeneathAttr struct {
	AllowedAccess uint64
	ParentFD      int32
	_             [4]byte
}

type helperConfig struct {
	WritableDirs []string `json:"w"`
}

func abi() int {
	ver, _, err := syscall.Syscall(landlockCreateRuleset, 0, 0, landlockCreateVersion)
	if err != 0 {
		return 0
	}
	return int(ver)
}

func landlockAccessForABI(abiVersion int) uint64 {
	mask := uint64(landlockAccessAllABI1)
	if abiVersion >= 2 {
		mask |= landlockAccessRefer
	}
	if abiVersion >= 3 {
		mask |= landlockAccessTruncate
	}
	return mask
}

func available() bool {
	_, _, err := syscall.Syscall(landlockCreateRuleset, 0, 0, landlockCreateVersion)
	return err == 0
}

func reasonUnavailable() string {
	if runtime.GOOS != "linux" {
		return "not Linux"
	}
	_, _, err := syscall.Syscall(landlockCreateRuleset, 0, 0, landlockCreateVersion)
	if err == syscall.ENOSYS {
		return "Landlock not supported (kernel < 5.13 or CONFIG_SECURITY_LANDLOCK=n)"
	}
	if err == syscall.EOPNOTSUPP {
		return "Landlock not enabled; add landlock=1 to kernel cmdline"
	}
	if err != 0 {
		return "Landlock error: " + err.Error()
	}
	return ""
}

func probeLinux() ProbeResult {
	r := ProbeResult{Platform: "linux"}
	v := abi()
	if v == 0 {
		r.Warning = "Landlock not available"
		r.Backend = "none"
		return r
	}
	r.Backend = fmt.Sprintf("landlock-abi%d", v)
	r.Sandboxed = true
	return r
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	// Skip JSON marshal when writable dirs are empty or only "."
	// to save allocation overhead for simple commands (ls, git status, etc.).
	var cfgJSON []byte
	if len(ctx.writable) == 0 || (len(ctx.writable) == 1 && ctx.writable[0] == ".") {
		cfgJSON = []byte(`{"w":null}`)
	} else {
		cfg := helperConfig{WritableDirs: ctx.writable}
		var err error
		cfgJSON, err = json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("sandbox: marshal config: %w", err)
		}
	}

	// Use cached executable path (set at init()) to avoid
	// repeated reads of /proc/self/exe for every command.
	if selfExePath == "" {
		return fmt.Errorf("sandbox: cannot find self path")
	}

	origPath := cmd.Path
	origArgs := cmd.Args

	// Rewrite to run via self-exec helper.
	cmd.Path = selfExePath
	newArgs := make([]string, 0, len(origArgs)+3)
	newArgs = append(newArgs, selfExePath, "__sandbox__", "--", origPath)
	newArgs = append(newArgs, origArgs[1:]...)
	cmd.Args = newArgs
	cmd.Env = append(cmd.Env,
		"__SANDBOX_HELPER=1",
		"__SANDBOX_CONFIG="+string(cfgJSON),
	)

	return nil
}

// setupLandlock applies Landlock filesystem rules in the current process.
// Called by the helper init() before exec.
func setupLandlock(cfg *helperConfig) error {
	v := abi()
	if v == 0 {
		return fmt.Errorf("Landlock not available")
	}

	handled := landlockAccessForABI(v)

	attr := landlockRulesetAttr{HandledAccessFS: handled}
	rulesetFd, _, err := syscall.Syscall(landlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		uintptr(unsafe.Sizeof(attr)),
		0,
	)
	if err != 0 {
		return fmt.Errorf("create ruleset: %w", err)
	}
	defer syscall.Close(int(rulesetFd))

	// Allow read+execute everywhere, block all writes.
	readAccess := handled &^ uint64(landlockAccessWriteABI1)
	if v >= 2 {
		readAccess &^= landlockAccessRefer
	}
	if v >= 3 {
		readAccess &^= landlockAccessTruncate
	}

	if err := addPathRule(int(rulesetFd), &landlockPathBeneathAttr{
		AllowedAccess: readAccess,
	}, "/"); err != nil {
		return fmt.Errorf("allow-read /: %w", err)
	}

	// Allow full access on writable directories.
	for _, dir := range cfg.WritableDirs {
		if err := addPathRule(int(rulesetFd), &landlockPathBeneathAttr{
			AllowedAccess: handled,
		}, dir); err != nil {
			return fmt.Errorf("allow-write %q: %w", dir, err)
		}
	}

	ret, _, err := syscall.Syscall(landlockRestrictSelf, uintptr(rulesetFd), 0, 0)
	if ret != 0 {
		return fmt.Errorf("restrict self: %w", err)
	}
	return nil
}

func addPathRule(rulesetFd int, ruleAttr *landlockPathBeneathAttr, dir string) error {
	dirFd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %q: %w", dir, err)
	}
	defer syscall.Close(dirFd)

	ruleAttr.ParentFD = int32(dirFd)
	ret, _, errno := syscall.Syscall(landlockAddRule,
		uintptr(rulesetFd),
		uintptr(landlockRulePathBeneath),
		uintptr(unsafe.Pointer(ruleAttr)),
	)
	if ret != 0 {
		return errno
	}
	return nil
}
