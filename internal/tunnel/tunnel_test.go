package tunnel

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestStartAndStopTunnel_SinglePort(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh not available, skipping integration test")
	}

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
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh not available, skipping integration test")
	}

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

func TestStart_AlreadyActive(t *testing.T) {
	tunnelsDir := t.TempDir()
	state := &TunnelState{
		Host: "p1", Service: "mysql", PID: os.Getpid(),
		LocalPort: 13306, RemoteHost: "127.0.0.1", RemotePort: 3306,
	}
	SaveState(StatePath(tunnelsDir, "p1", "mysql"), state)

	_, err := Start(&StartOptions{
		Host: "p1", Service: "mysql",
		Ports:          []PortForward{{RemoteHost: "127.0.0.1", RemotePort: 3306}},
		TunnelsDir:     tunnelsDir,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("expected error for already-active tunnel")
	}
}

func TestStart_OrphanedStateCleanup(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh not available")
	}
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
