package tunnel

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestIsProcessAlive_Self(t *testing.T) {
	pid := os.Getpid()
	if !IsProcessAlive(pid) {
		t.Error("current process should be alive")
	}
}

func TestIsProcessAlive_Invalid(t *testing.T) {
	if IsProcessAlive(999999999) {
		t.Error("invalid PID should not be alive")
	}
}

func TestIsTunnelAlive_CurrentProcessIsNotSSH(t *testing.T) {
	// The current test process is alive but is not ssh → must not be treated as a tunnel.
	if IsTunnelAlive(os.Getpid()) {
		t.Error("current test process is alive but not ssh; IsTunnelAlive must be false")
	}
}

func TestIsTunnelAlive_DeadPID(t *testing.T) {
	// A PID that almost certainly does not exist.
	if IsTunnelAlive(0x7FFFFFF0) {
		t.Error("nonexistent PID must not be reported alive")
	}
}

// startSleeper spawns a child process that stays alive until it is killed, so
// tests have a real PID to terminate. It re-executes the test binary, which is
// the only portable way to get a long-lived process here.
func startSleeper(t *testing.T) int {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperSleeper", "--")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cannot start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // reap, so the helper never lingers as a zombie
	})
	return cmd.Process.Pid
}

const helperEnv = "GO_SSHFORWARD_HELPER"

// TestHelperSleeper is not a test: it is the body of the child process started
// by startSleeper, and does nothing unless that helper env var is set.
func TestHelperSleeper(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process body, started only by startSleeper")
	}
	time.Sleep(2 * time.Minute)
}

func TestTerminateAndWait_ProcessIsGoneOnReturn(t *testing.T) {
	pid := startSleeper(t)
	if !IsProcessAlive(pid) {
		t.Fatalf("helper process %d should be alive before termination", pid)
	}

	if err := TerminateAndWait(pid); err != nil {
		t.Fatalf("TerminateAndWait(%d) failed: %v", pid, err)
	}

	// The contract is that the process is gone the moment this returns, not
	// merely that a signal was delivered.
	if IsProcessAlive(pid) {
		t.Errorf("process %d still reported alive after TerminateAndWait returned", pid)
	}
}

func TestIsProcessAlive_ExitedChildIsDead(t *testing.T) {
	// An exited child that the parent has not reaped is a zombie on Unix, and
	// stays openable by PID on Windows while a handle is held. Neither counts
	// as alive, or stop would wait for an exit that already happened.
	pid := startSleeper(t)
	if err := KillProcess(pid); err != nil {
		t.Fatalf("cannot kill helper process %d: %v", pid, err)
	}
	if !waitForExit(pid, 5*time.Second) {
		t.Fatalf("helper process %d still reported alive 5s after being killed", pid)
	}
}
