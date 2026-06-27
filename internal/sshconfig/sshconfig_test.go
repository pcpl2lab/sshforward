package sshconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListHosts(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	content := `Host p1
    HostName 192.168.1.100
    User admin

Host p2
    HostName 192.168.1.101

Host *
    ServerAliveInterval 60
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	hosts, err := ListHostsFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := map[string]bool{"p1": true, "p2": true}
	if len(hosts) != len(expected) {
		t.Fatalf("expected %d hosts, got %d: %v", len(expected), len(hosts), hosts)
	}
	for _, h := range hosts {
		if !expected[h] {
			t.Errorf("unexpected host: %s", h)
		}
	}
}

func TestValidateHost_Exists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	content := `Host p1
    HostName 192.168.1.100
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	err := ValidateHostFromPath(cfgPath, "p1")
	if err != nil {
		t.Errorf("expected no error for existing host, got: %v", err)
	}
}

func TestValidateHost_NotExists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	content := `Host p1
    HostName 192.168.1.100
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	err := ValidateHostFromPath(cfgPath, "unknown")
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
}
