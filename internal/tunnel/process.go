package tunnel

import (
	"fmt"
	"time"
)

// Process helpers. IsProcessAlive / IsSSHProcess / KillProcess /
// ForceKillProcess have platform-specific implementations in process_unix.go
// and process_windows.go.

// Timings for TerminateAndWait. The grace period is what a healthy ssh needs to
// close its channels and exit; the force period only covers the kernel tearing
// down a process that has already been killed outright.
const (
	stopGracePeriod  = 3 * time.Second
	stopForcePeriod  = 2 * time.Second
	stopPollInterval = 50 * time.Millisecond
)

// IsTunnelAlive reports whether pid belongs to a live ssh process. Combining
// liveness with an image-name check guards against PID recycling: after a
// reboot or PID wraparound an unrelated process may hold a stored PID.
func IsTunnelAlive(pid int) bool {
	return IsProcessAlive(pid) && IsSSHProcess(pid)
}

// TerminateAndWait stops the process and does not return until it is gone,
// escalating to a forced kill if the graceful signal is ignored. Callers rely
// on this so that a tunnel's local port is actually released — and its state
// file only deleted — once the process has really exited.
func TerminateAndWait(pid int) error {
	if err := KillProcess(pid); err != nil {
		return fmt.Errorf("cannot stop process %d: %w", pid, err)
	}
	if waitForExit(pid, stopGracePeriod) {
		return nil
	}

	if err := ForceKillProcess(pid); err != nil {
		return fmt.Errorf("process %d ignored the stop signal and could not be killed: %w", pid, err)
	}
	if waitForExit(pid, stopForcePeriod) {
		return nil
	}
	return fmt.Errorf("process %d is still running after a forced kill", pid)
}

// waitForExit polls until pid is gone, reporting whether it exited in time.
func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !IsProcessAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(stopPollInterval)
	}
}
