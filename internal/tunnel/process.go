package tunnel

import (
	"fmt"
	"time"
)

// Process helpers. IsProcessAlive / IsSSHProcess / KillProcess /
// ForceKillProcess / processStartTime have platform-specific implementations in
// process_unix.go and process_windows.go.

// Timings for TerminateAndWait. The grace period is what a healthy ssh needs to
// close its channels and exit; the force period only covers the kernel tearing
// down a process that has already been killed outright.
const (
	stopGracePeriod  = 3 * time.Second
	stopForcePeriod  = 2 * time.Second
	stopPollInterval = 50 * time.Millisecond
)

// startTimeTolerance absorbs the coarse clocks some platforms report process
// creation with: whole seconds on macOS, a boot time rounded to the second on
// Linux.
const startTimeTolerance = 2 * time.Second

// IsTunnelAlive reports whether pid belongs to a live ssh process. Combining
// liveness with an image-name check guards against PID recycling: after a
// reboot or PID wraparound an unrelated process may hold a stored PID.
func IsTunnelAlive(pid int) bool {
	return IsProcessAlive(pid) && IsSSHProcess(pid)
}

// IsAlive reports whether the tunnel recorded in s is still running. The
// image-name check in IsTunnelAlive is not enough on its own: after a reboot the
// stored PID can be handed to an unrelated ssh — an interactive session, git, an
// editor's remote helper — which list would then show as active, start would
// refuse to replace, and stop would kill. Such a process is necessarily created
// after the tunnel was recorded, so its creation time gives it away. This needs
// nothing beyond StartedAt, so it also covers state files written by older
// versions.
func (s *TunnelState) IsAlive() bool {
	if !IsTunnelAlive(s.PID) {
		return false
	}
	created, err := processStartTime(s.PID)
	if err != nil {
		return true // cannot tell; fall back to the PID and image-name check
	}
	return !created.After(s.StartedAt.Add(startTimeTolerance))
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
