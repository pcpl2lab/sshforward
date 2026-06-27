//go:build !windows

package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func IsProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
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

func KillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
