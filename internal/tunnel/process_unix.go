//go:build !windows

package tunnel

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false // state files are user-editable; negative pid would signal a process group
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	// A zombie keeps answering signal 0 until its parent reaps it, but the
	// tunnel is already gone. Reporting it alive would make stop wait for an
	// exit that has in fact already happened.
	return !isZombie(pid)
}

// isZombie reports whether pid has exited but has not been reaped yet.
func isZombie(pid int) bool {
	// Linux: /proc/<pid>/stat is "<pid> (<comm>) <state> ...". comm may contain
	// spaces and parentheses, so the state letter is found after the last ')'.
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		i := bytes.LastIndexByte(data, ')')
		if i < 0 {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(string(data[i+1:])), "Z")
	}
	// macOS and friends: ask ps for the state code.
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "state=").Output()
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

func IsSSHProcess(pid int) bool {
	// Try /proc first (Linux)
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		// /proc not available (macOS) — fallback to ps
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err != nil {
			return false
		}
		name := strings.TrimSpace(string(out))
		return strings.Contains(name, "ssh")
	}
	name := strings.TrimSpace(string(data))
	return name == "ssh"
}

// KillProcess asks the process to terminate, giving ssh a chance to tear the
// connection down cleanly.
func KillProcess(pid int) error {
	return signalProcess(pid, syscall.SIGTERM)
}

// ForceKillProcess terminates the process outright. It is the escalation for a
// process that ignored KillProcess.
func ForceKillProcess(pid int) error {
	return signalProcess(pid, syscall.SIGKILL)
}

func signalProcess(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}
