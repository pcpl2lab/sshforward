package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestNonInteractive_CommandsUnaffected runs the real binary without a
// terminal: the commands that never needed one must behave as before, and the
// interactive ones must fail fast instead of waiting for input.
func TestNonInteractive_CommandsUnaffected(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}

	bin := filepath.Join(t.TempDir(), "sshforward")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	home := t.TempDir()
	dir := filepath.Join(home, ".sshforward")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	const configSrc = "services:\n  mysql:\n    remote_port: 3306\n"
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configSrc), 0o600); err != nil {
		t.Fatal(err)
	}

	runIn := func(t *testing.T, home string, args ...string) (stdout, stderr string, code int, elapsed time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		devNull, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		defer devNull.Close()

		var out, errOut bytes.Buffer
		c := exec.CommandContext(ctx, bin, args...)
		c.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, noUpdateCheckEnv+"=1")
		c.Stdin = devNull
		c.Stdout = &out
		c.Stderr = &errOut

		start := time.Now()
		err = c.Run()
		elapsed = time.Since(start)
		if ctx.Err() != nil {
			t.Fatalf("%v did not finish within the timeout", args)
		}
		var exitErr *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &exitErr):
			code = exitErr.ExitCode()
		default:
			t.Fatalf("%v: %v", args, err)
		}
		return out.String(), errOut.String(), code, elapsed
	}
	run := func(t *testing.T, args ...string) (string, string, int, time.Duration) {
		t.Helper()
		return runIn(t, home, args...)
	}

	for _, args := range [][]string{{"version"}, {"config"}, {"list"}} {
		t.Run(args[0], func(t *testing.T) {
			stdout, stderr, code, _ := run(t, args...)
			if code != 0 {
				t.Fatalf("exit code %d, stderr: %s", code, stderr)
			}
			if strings.Contains(stdout+stderr, "\x1b[") {
				t.Errorf("output contains an escape sequence: %q", stdout+stderr)
			}
			if args[0] == "config" && !strings.Contains(stdout, "mysql") {
				t.Errorf("config output does not list the service: %q", stdout)
			}
		})
	}

	// Only the stdin half of requireTerminal is exercised: stdout is a pipe
	// here too, and a terminal stdout with a redirected stdin needs a pty.
	for _, name := range []string{"add", "manage"} {
		t.Run(name, func(t *testing.T) {
			if cmd, _, err := rootCmd.Find([]string{name}); err != nil || cmd == rootCmd {
				t.Skipf("%s is not registered yet", name)
			}
			_, stderr, code, elapsed := run(t, name)
			if code != 1 {
				t.Fatalf("exit code %d, want 1; stderr: %s", code, stderr)
			}
			if !strings.Contains(stderr, "sshforward edit") {
				t.Errorf("stderr = %q, want it to point to 'sshforward edit'", stderr)
			}
			if elapsed > 5*time.Second {
				t.Errorf("took %v, want a fast failure", elapsed)
			}
			if data, err := os.ReadFile(configPath); err != nil || string(data) != configSrc {
				t.Errorf("config changed: %q, %v", data, err)
			}
		})
	}

	t.Run("add without config", func(t *testing.T) {
		empty := t.TempDir()
		_, stderr, code, _ := runIn(t, empty, "add")
		if code != 1 || !strings.Contains(stderr, "sshforward edit") {
			t.Fatalf("exit code %d, stderr %q; want 1 and a pointer to 'sshforward edit'", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(empty, ".sshforward", "config.yaml")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("config file was created (stat err: %v)", err)
		}
	})
}
