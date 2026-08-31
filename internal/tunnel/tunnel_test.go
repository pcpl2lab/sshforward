package tunnel

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// TestMain doubles as the body of the stand-in ssh client. When this binary has
// been copied to a file named "ssh" it must behave like a long-lived tunnel
// process rather than run the test suite. The check is on the executable's own
// name because the child inherits the parent's environment, so an env var would
// make the parent match too. flag parsing happens inside m.Run, so ssh's
// arguments never reach it.
func TestMain(m *testing.M) {
	if base := filepath.Base(os.Args[0]); base == "ssh" || base == "ssh.exe" {
		fakeSSH()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeSSH mimics the parts of `ssh -N -E <log> -L ...` that Start observes: it
// creates the log file and then stays alive until it is killed.
func fakeSSH() {
	for i, a := range os.Args {
		if a == "-E" && i+1 < len(os.Args) {
			_ = os.WriteFile(os.Args[i+1], []byte("stand-in ssh client\n"), 0o600)
		}
	}
	time.Sleep(2 * time.Minute)
}

// useFakeSSH points Start at the stand-in for the duration of the test.
//
// The real client cannot be used for start/stop tests: on any machine running
// sshd — every GitHub runner — it is rejected and exits within milliseconds, so
// the liveness check sees a dead process; on a machine without one it lingers
// long enough to pass. The outcome would depend on the environment instead of
// on the code. TestBuildSSHArgs_OptionsAcceptedByRealSSH covers the real client
// separately, without needing a server.
func useFakeSSH(t *testing.T) {
	t.Helper()

	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Skipf("cannot read the test binary to build a stand-in ssh: %v", err)
	}
	name := "ssh"
	if runtime.GOOS == "windows" {
		name = "ssh.exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, self, 0o755); err != nil {
		t.Fatalf("write stand-in ssh: %v", err)
	}

	previous := lookSSHPath
	lookSSHPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { lookSSHPath = previous })
}

func TestStartAndStopTunnel_SinglePort(t *testing.T) {
	useFakeSSH(t)

	tunnelsDir := t.TempDir()

	opts := &StartOptions{
		Host:    "localhost",
		Service: "test",
		Ports: []PortForward{{
			RemoteHost: "127.0.0.1",
			RemotePort: 22,
		}},
		TunnelsDir:     tunnelsDir,
		SkipValidation: true,
	}

	states, err := Start(opts)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if states[0].PID == 0 {
		t.Error("expected non-zero PID")
	}
	if states[0].LocalPort == 0 {
		t.Error("expected non-zero local port")
	}

	statePath := StatePathWithPort(tunnelsDir, "localhost", "test", "")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state file not created")
	}

	// Check log file was created
	logPath := LogPath(tunnelsDir, "localhost", "test", "")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file not created")
	}

	err = Stop(tunnelsDir, "localhost", "test")
	if err != nil {
		t.Errorf("stop failed: %v", err)
	}

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("state file not removed after stop")
	}
}

func TestStartAndStopTunnel_MultiPort(t *testing.T) {
	useFakeSSH(t)

	tunnelsDir := t.TempDir()

	opts := &StartOptions{
		Host:    "localhost",
		Service: "gitea",
		Ports: []PortForward{
			{Name: "web", RemoteHost: "127.0.0.1", RemotePort: 22},
			{Name: "ssh", RemoteHost: "127.0.0.1", RemotePort: 22},
		},
		TunnelsDir:     tunnelsDir,
		SkipValidation: true,
	}

	states, err := Start(opts)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}

	// Check both state files exist
	for _, name := range []string{"web", "ssh"} {
		p := StatePathWithPort(tunnelsDir, "localhost", "gitea", name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("state file for port %q not created", name)
		}
	}

	// Stop should kill both
	err = Stop(tunnelsDir, "localhost", "gitea")
	if err != nil {
		t.Errorf("stop failed: %v", err)
	}

	for _, name := range []string{"web", "ssh"} {
		p := StatePathWithPort(tunnelsDir, "localhost", "gitea", name)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("state file for port %q not removed after stop", name)
		}
	}
}

func TestStop_NonExistentTunnel(t *testing.T) {
	tunnelsDir := t.TempDir()
	err := Stop(tunnelsDir, "nohost", "noservice")
	if err == nil {
		t.Fatal("expected error for non-existent tunnel")
	}
}

func TestStart_LiveNonSSHPIDIsStale(t *testing.T) {
	useFakeSSH(t)

	tunnelsDir := t.TempDir()
	// A PID that is alive but is not ssh: after a reboot or PID wraparound an
	// unrelated process can inherit a stored PID. That must not block a start.
	state := &TunnelState{
		Host: "localhost", Service: "test", PID: os.Getpid(),
		LocalPort: 13306, RemoteHost: "127.0.0.1", RemotePort: 22,
	}
	statePath := StatePath(tunnelsDir, "localhost", "test")
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	states, err := Start(&StartOptions{
		Host: "localhost", Service: "test",
		Ports:          []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 22}},
		TunnelsDir:     tunnelsDir,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("start over a recycled PID should succeed, got: %v", err)
	}
	t.Cleanup(func() { _ = Stop(tunnelsDir, "localhost", "test") })

	if len(states) != 1 {
		t.Fatalf("got %d states, want 1", len(states))
	}
	if states[0].PID == os.Getpid() {
		t.Errorf("state still holds the stale PID %d, want the new ssh PID", states[0].PID)
	}
}

func TestStart_OrphanedStateCleanup(t *testing.T) {
	useFakeSSH(t)

	tunnelsDir := t.TempDir()
	state := &TunnelState{
		Host: "localhost", Service: "test", PID: 999999999,
		LocalPort: 19999, RemoteHost: "127.0.0.1", RemotePort: 22,
	}
	statePath := StatePath(tunnelsDir, "localhost", "test")
	SaveState(statePath, state)

	states, err := Start(&StartOptions{
		Host: "localhost", Service: "test",
		Ports:          []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 22}},
		TunnelsDir:     tunnelsDir,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("start failed despite orphaned state: %v", err)
	}
	Stop(tunnelsDir, "localhost", "test")
	_ = states
}

func TestStart_FixedPortOccupied(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	occupiedPort, _ := strconv.Atoi(portStr)

	tunnelsDir := t.TempDir()
	_, err := Start(&StartOptions{
		Host: "p1", Service: "mysql",
		Ports:      []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 3306, LocalPort: occupiedPort}},
		TunnelsDir: tunnelsDir,
	})
	if err == nil {
		t.Fatal("expected error for occupied port")
	}
}

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		ports   []PortForward
		wantErr bool
	}{
		{"valid host", "p1", []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 22}}, false},
		{"valid host with dots", "my.server.com", []PortForward{{RemoteHost: "10.0.0.1", RemotePort: 80}}, false},
		{"host with spaces", "bad host", []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 22}}, true},
		{"host with semicolon", "host;rm -rf /", []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 22}}, true},
		{"invalid remote_host", "p1", []PortForward{{RemoteHost: "bad host", RemotePort: 22}}, true},
		{"port out of range", "p1", []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 0}}, true},
		{"port too high", "p1", []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 70000}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInput(tt.host, tt.ports)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLogPath(t *testing.T) {
	dir := t.TempDir()
	got := LogPath(dir, "p1", "gitea", "web")
	expected := filepath.Join(dir, "p1-gitea-web.log")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestBuildSSHArgs_OptionsAcceptedByRealSSH(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh not available, skipping option check")
	}

	// -G makes ssh evaluate its configuration and print it without connecting,
	// which checks every -o key against the installed OpenSSH. A misspelled
	// option would otherwise only show up as a tunnel that refuses to start,
	// and only on a user's machine. -F points at an empty config so the
	// developer's own ~/.ssh/config cannot influence the result.
	emptyConfig := filepath.Join(t.TempDir(), "empty_ssh_config")
	if err := os.WriteFile(emptyConfig, nil, 0o600); err != nil {
		t.Fatalf("write empty ssh config: %v", err)
	}

	built := buildSSHArgs(filepath.Join(t.TempDir(), "ssh.log"), "13306:127.0.0.1:3306", "localhost")
	args := []string{"-G", "-F", emptyConfig}
	for i, a := range built {
		if a == "-o" && i+1 < len(built) {
			args = append(args, "-o", built[i+1])
		}
	}
	args = append(args, "localhost")

	out, err := exec.Command(sshPath, args...).CombinedOutput()
	if err != nil {
		t.Errorf("OpenSSH rejected the options sshforward passes: %v\n%s", err, out)
	}
}
