package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/pcpl2lab/sshforward/internal/tunnel"
)

func TestConfigLoadAndServiceLookup_SinglePort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`services:
  mysql:
    remote_port: 3306
  redis:
    remote_port: 6379
    local_port: 16379
`), 0644)

	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	mysql, err := cfg.GetService("mysql")
	if err != nil {
		t.Fatalf("mysql not found: %v", err)
	}
	mp := mysql.Ports[0]
	// No local_port → defaults to remote_port
	if mp.RemoteHost != "127.0.0.1" || mp.RemotePort != 3306 || mp.LocalPort != 3306 {
		t.Errorf("unexpected mysql config: %+v", mp)
	}

	redis, _ := cfg.GetService("redis")
	if redis.Ports[0].LocalPort != 16379 {
		t.Errorf("expected redis local_port 16379, got %d", redis.Ports[0].LocalPort)
	}

	_, err = cfg.GetService("nonexistent")
	if err == nil {
		t.Error("expected error for unknown service")
	}
}

func TestConfigLoadAndServiceLookup_MultiPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`services:
  gitea:
    remote_host: 10.0.0.5
    ports:
      - name: web
        remote_port: 3000
      - name: ssh
        remote_port: 2222
        local_port: 12222
`), 0644)

	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	gitea, err := cfg.GetService("gitea")
	if err != nil {
		t.Fatalf("gitea not found: %v", err)
	}
	if !gitea.IsMultiPort() {
		t.Error("gitea should be multi-port")
	}
	// web: no local_port → defaults to remote_port 3000
	if gitea.Ports[0].LocalPort != 3000 {
		t.Errorf("expected web local_port 3000, got %d", gitea.Ports[0].LocalPort)
	}
	// ssh: explicit local_port 12222
	if gitea.Ports[1].LocalPort != 12222 {
		t.Errorf("expected ssh local_port 12222, got %d", gitea.Ports[1].LocalPort)
	}
}

func TestTunnelStateLifecycle(t *testing.T) {
	dir := t.TempDir()

	states, _ := tunnel.ListStates(dir)
	if len(states) != 0 {
		t.Fatalf("expected 0 states, got %d", len(states))
	}

	state := &tunnel.TunnelState{
		Host: "p1", Service: "mysql", PID: os.Getpid(),
		LocalPort: 13306, RemoteHost: "127.0.0.1", RemotePort: 3306,
	}
	path := tunnel.StatePath(dir, "p1", "mysql")
	tunnel.SaveState(path, state)

	states, _ = tunnel.ListStates(dir)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}

	tunnel.RemoveState(path)
	states, _ = tunnel.ListStates(dir)
	if len(states) != 0 {
		t.Fatalf("expected 0 states after removal, got %d", len(states))
	}
}

func TestTunnelStateLifecycle_MultiPort(t *testing.T) {
	dir := t.TempDir()

	s1 := &tunnel.TunnelState{
		Host: "p1", Service: "gitea", PortName: "web", PID: os.Getpid(),
		LocalPort: 3000, RemoteHost: "10.0.0.5", RemotePort: 3000,
	}
	s2 := &tunnel.TunnelState{
		Host: "p1", Service: "gitea", PortName: "ssh", PID: os.Getpid(),
		LocalPort: 2222, RemoteHost: "10.0.0.5", RemotePort: 2222,
	}

	tunnel.SaveState(tunnel.StatePathWithPort(dir, "p1", "gitea", "web"), s1)
	tunnel.SaveState(tunnel.StatePathWithPort(dir, "p1", "gitea", "ssh"), s2)

	giteaStates, _ := tunnel.ListServiceStates(dir, "p1", "gitea")
	if len(giteaStates) != 2 {
		t.Fatalf("expected 2 gitea states, got %d", len(giteaStates))
	}
}
