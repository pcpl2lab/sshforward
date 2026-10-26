//go:build windows

package tunnel

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// stillActive is the Win32 STILL_ACTIVE status GetExitCodeProcess reports for a
// process that has not exited yet.
const stillActive = 259

func IsProcessAlive(pid int) bool {
	// State files are user-editable, so reject PIDs outside the DWORD range that
	// OpenProcess takes. The uint64 conversion is what makes this compile on
	// 386, where int is 32 bits and cannot be compared to MaxUint32 directly.
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	// A terminated process stays openable by PID for as long as anything still
	// holds a handle to it, so OpenProcess succeeding proves nothing. Liveness
	// has to come from the exit code.
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

func IsSSHProcess(pid int) bool {
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var buf [windows.MAX_PATH]uint16
	size := uint32(len(buf))
	err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
	if err != nil {
		return false
	}
	name := filepath.Base(windows.UTF16ToString(buf[:size]))
	return strings.EqualFold(name, "ssh.exe")
}

// processStartTime returns when the process was created. Windows records it as
// wall-clock time at creation, so later clock adjustments do not shift it.
func processStartTime(pid int) (time.Time, error) {
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return time.Time{}, fmt.Errorf("invalid pid %d", pid)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}, err
	}
	defer windows.CloseHandle(h)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}

// KillProcess terminates the process. Windows has no graceful equivalent here:
// the tunnel is created with DETACHED_PROCESS, so it owns no console and cannot
// be reached by GenerateConsoleCtrlEvent — TerminateProcess is the only option.
func KillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// ForceKillProcess exists so callers can escalate on every platform. On Windows
// KillProcess is already forceful, so this is the same call.
func ForceKillProcess(pid int) error {
	return KillProcess(pid)
}
