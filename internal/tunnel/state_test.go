package tunnel

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	state := &TunnelState{
		Host:       "p1",
		Service:    "mysql",
		PID:        12345,
		LocalPort:  13306,
		RemoteHost: "127.0.0.1",
		RemotePort: 3306,
		StartedAt:  time.Now().UTC().Truncate(time.Second),
	}

	path := filepath.Join(dir, "p1-mysql.json")
	if err := SaveState(path, state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Host != state.Host || loaded.Service != state.Service {
		t.Errorf("host/service mismatch")
	}
	if loaded.PID != state.PID || loaded.LocalPort != state.LocalPort {
		t.Errorf("pid/port mismatch")
	}
	if !loaded.StartedAt.Equal(state.StartedAt) {
		t.Errorf("started_at mismatch: %v vs %v", loaded.StartedAt, state.StartedAt)
	}
}

func TestSaveAndLoadState_WithPortName(t *testing.T) {
	dir := t.TempDir()
	state := &TunnelState{
		Host:       "p1",
		Service:    "gitea",
		PortName:   "web",
		PID:        12345,
		LocalPort:  13000,
		RemoteHost: "10.0.0.5",
		RemotePort: 3000,
		StartedAt:  time.Now().UTC().Truncate(time.Second),
	}

	path := StatePathWithPort(dir, "p1", "gitea", "web")
	if err := SaveState(path, state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.PortName != "web" {
		t.Errorf("expected port_name 'web', got %q", loaded.PortName)
	}
}

func TestStateKey(t *testing.T) {
	if got := StateKey("p1", "mysql", ""); got != "p1-mysql" {
		t.Errorf("expected p1-mysql, got %s", got)
	}
	if got := StateKey("p1", "gitea", "web"); got != "p1-gitea-web" {
		t.Errorf("expected p1-gitea-web, got %s", got)
	}
}

func TestStatePath(t *testing.T) {
	dir := "/home/user/.sshforward/tunnels"
	got := StatePath(dir, "p1", "mysql")
	expected := filepath.Join(dir, "p1-mysql.json")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestStatePathWithPort(t *testing.T) {
	dir := "/home/user/.sshforward/tunnels"
	got := StatePathWithPort(dir, "p1", "gitea", "web")
	expected := filepath.Join(dir, "p1-gitea-web.json")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestListStates_Empty(t *testing.T) {
	dir := t.TempDir()
	states, err := ListStates(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 states, got %d", len(states))
	}
}

func TestListStates_WithFiles(t *testing.T) {
	dir := t.TempDir()
	s1 := &TunnelState{Host: "p1", Service: "mysql", PID: 1, LocalPort: 3306, RemoteHost: "127.0.0.1", RemotePort: 3306, StartedAt: time.Now()}
	s2 := &TunnelState{Host: "p1", Service: "redis", PID: 2, LocalPort: 6379, RemoteHost: "127.0.0.1", RemotePort: 6379, StartedAt: time.Now()}
	SaveState(StatePath(dir, "p1", "mysql"), s1)
	SaveState(StatePath(dir, "p1", "redis"), s2)

	states, err := ListStates(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
}

func TestListServiceStates(t *testing.T) {
	dir := t.TempDir()
	s1 := &TunnelState{Host: "p1", Service: "gitea", PortName: "web", PID: 1, LocalPort: 3000, RemoteHost: "10.0.0.5", RemotePort: 3000, StartedAt: time.Now()}
	s2 := &TunnelState{Host: "p1", Service: "gitea", PortName: "ssh", PID: 2, LocalPort: 2222, RemoteHost: "10.0.0.5", RemotePort: 2222, StartedAt: time.Now()}
	s3 := &TunnelState{Host: "p1", Service: "mysql", PID: 3, LocalPort: 3306, RemoteHost: "127.0.0.1", RemotePort: 3306, StartedAt: time.Now()}

	SaveState(StatePathWithPort(dir, "p1", "gitea", "web"), s1)
	SaveState(StatePathWithPort(dir, "p1", "gitea", "ssh"), s2)
	SaveState(StatePath(dir, "p1", "mysql"), s3)

	gitea, err := ListServiceStates(dir, "p1", "gitea")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gitea) != 2 {
		t.Fatalf("expected 2 gitea states, got %d", len(gitea))
	}

	mysql, _ := ListServiceStates(dir, "p1", "mysql")
	if len(mysql) != 1 {
		t.Fatalf("expected 1 mysql state, got %d", len(mysql))
	}
}
