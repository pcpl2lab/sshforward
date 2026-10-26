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
	"time"
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

// userHZ is the unit of the start time in /proc/<pid>/stat. The kernel exports
// it to userspace as a fixed 100 regardless of its internal tick rate.
const userHZ = 100

// processStartTime returns when the process was created.
func processStartTime(pid int) (time.Time, error) {
	if pid <= 0 {
		return time.Time{}, fmt.Errorf("invalid pid %d", pid)
	}
	// Linux: field 22 of /proc/<pid>/stat is the start time in ticks since
	// boot; btime in /proc/stat anchors boot to the wall clock.
	if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		sys, err := os.ReadFile("/proc/stat")
		if err != nil {
			return time.Time{}, err
		}
		return procStartTime(stat, sys)
	}
	// macOS and friends: ps prints the start time in ctime(3) format. LC_ALL=C
	// keeps the day and month names parseable.
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(string(out)), " "), time.Local)
}

// procStartTime computes a process start time from the contents of
// /proc/<pid>/stat and /proc/stat.
func procStartTime(stat, sys []byte) (time.Time, error) {
	// comm may contain spaces and parentheses, so fields are counted after the
	// last ')'. The first of them is field 3 (state), making starttime the 20th.
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 {
		return time.Time{}, fmt.Errorf("malformed /proc stat")
	}
	fields := strings.Fields(string(stat[i+1:]))
	if len(fields) < 20 {
		return time.Time{}, fmt.Errorf("malformed /proc stat")
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("malformed /proc stat start time: %w", err)
	}

	for line := range strings.Lines(string(sys)) {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			boot, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("malformed btime: %w", err)
			}
			return time.Unix(boot, 0).Add(time.Duration(ticks) * time.Second / userHZ), nil
		}
	}
	return time.Time{}, fmt.Errorf("btime not found in /proc/stat")
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
