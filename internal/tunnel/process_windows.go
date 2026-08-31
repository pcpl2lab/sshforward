//go:build windows

package tunnel

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// stillActive is the Win32 STILL_ACTIVE status GetExitCodeProcess reports for a
// process that has not exited yet.
const stillActive = 259

func IsProcessAlive(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false // state files are user-editable; reject PIDs outside DWORD range
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
	if pid <= 0 || pid > math.MaxUint32 {
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
