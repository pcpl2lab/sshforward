package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_SinglePort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  mysql:
    remote_host: 127.0.0.1
    remote_port: 3306
    local_port: 13306
  redis:
    remote_port: 6379
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}

	mysql := cfg.Services["mysql"]
	if mysql.IsMultiPort() {
		t.Error("mysql should not be multi-port")
	}
	mp := mysql.Ports[0]
	if mp.RemoteHost != "127.0.0.1" {
		t.Errorf("expected remote_host 127.0.0.1, got %s", mp.RemoteHost)
	}
	if mp.RemotePort != 3306 {
		t.Errorf("expected remote_port 3306, got %d", mp.RemotePort)
	}
	if mp.LocalPort != 13306 {
		t.Errorf("expected local_port 13306, got %d", mp.LocalPort)
	}

	// redis: no local_port specified → should default to remote_port (6379)
	redis := cfg.Services["redis"]
	rp := redis.Ports[0]
	if rp.RemoteHost != "127.0.0.1" {
		t.Errorf("expected default remote_host 127.0.0.1, got %s", rp.RemoteHost)
	}
	if rp.LocalPort != 6379 {
		t.Errorf("expected local_port to default to remote_port 6379, got %d", rp.LocalPort)
	}
}

func TestLoadConfig_LocalPortDefaultsToRemotePort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  mysql:
    remote_port: 3306
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Services["mysql"].Ports[0].LocalPort != 3306 {
		t.Errorf("expected local_port to default to 3306, got %d", cfg.Services["mysql"].Ports[0].LocalPort)
	}
}

func TestLoadConfig_LocalPortAutoIsRandom(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  mysql:
    remote_port: 3306
    local_port: auto
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Services["mysql"].Ports[0].LocalPort != 0 {
		t.Errorf("expected local_port 0 for 'auto', got %d", cfg.Services["mysql"].Ports[0].LocalPort)
	}
}

func TestLoadConfig_LocalPortZeroIsAuto(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  mysql:
    remote_port: 3306
    local_port: 0
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Services["mysql"].Ports[0].LocalPort != 0 {
		t.Errorf("expected local_port 0 for '0', got %d", cfg.Services["mysql"].Ports[0].LocalPort)
	}
}

func TestLoadConfig_MultiPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  gitea:
    remote_host: 10.0.0.5
    ports:
      - name: web
        remote_port: 3000
      - name: ssh
        remote_port: 2222
        local_port: 12222
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gitea := cfg.Services["gitea"]
	if !gitea.IsMultiPort() {
		t.Error("gitea should be multi-port")
	}
	if len(gitea.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(gitea.Ports))
	}

	web := gitea.Ports[0]
	if web.Name != "web" {
		t.Errorf("expected name 'web', got %q", web.Name)
	}
	if web.RemoteHost != "10.0.0.5" {
		t.Errorf("expected remote_host 10.0.0.5, got %s", web.RemoteHost)
	}
	if web.RemotePort != 3000 {
		t.Errorf("expected remote_port 3000, got %d", web.RemotePort)
	}
	// No local_port → default to remote_port (3000)
	if web.LocalPort != 3000 {
		t.Errorf("expected local_port to default to 3000, got %d", web.LocalPort)
	}

	ssh := gitea.Ports[1]
	if ssh.Name != "ssh" {
		t.Errorf("expected name 'ssh', got %q", ssh.Name)
	}
	if ssh.RemotePort != 2222 {
		t.Errorf("expected remote_port 2222, got %d", ssh.RemotePort)
	}
	if ssh.LocalPort != 12222 {
		t.Errorf("expected local_port 12222, got %d", ssh.LocalPort)
	}
}

func TestLoadConfig_MultiPortAutoLocalPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  svc:
    ports:
      - name: a
        remote_port: 80
        local_port: auto
      - name: b
        remote_port: 443
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := cfg.Services["svc"]
	if svc.Ports[0].LocalPort != 0 {
		t.Errorf("port 'a': expected auto (0), got %d", svc.Ports[0].LocalPort)
	}
	if svc.Ports[1].LocalPort != 443 {
		t.Errorf("port 'b': expected default 443, got %d", svc.Ports[1].LocalPort)
	}
}

func TestLoadConfig_MultiPortOverrideRemoteHost(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  multi:
    remote_host: 10.0.0.1
    ports:
      - name: a
        remote_port: 80
        remote_host: 10.0.0.2
      - name: b
        remote_port: 443
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := cfg.Services["multi"]
	if svc.Ports[0].RemoteHost != "10.0.0.2" {
		t.Errorf("port 'a' should override remote_host, got %s", svc.Ports[0].RemoteHost)
	}
	if svc.Ports[1].RemoteHost != "10.0.0.1" {
		t.Errorf("port 'b' should inherit remote_host, got %s", svc.Ports[1].RemoteHost)
	}
}

func TestLoadConfig_MixedSingleAndMultiPortError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  bad:
    remote_port: 3306
    ports:
      - name: web
        remote_port: 3000
      - name: ssh
        remote_port: 2222
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadFromPath(cfgPath)
	if err == nil {
		t.Fatal("expected error when both remote_port and ports are set")
	}
}

func TestLoadConfig_MultiPortDuplicateName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  bad:
    ports:
      - name: web
        remote_port: 80
      - name: web
        remote_port: 443
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadFromPath(cfgPath)
	if err == nil {
		t.Fatal("expected error for duplicate port name")
	}
}

func TestLoadConfig_MultiPortMissingName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  bad:
    ports:
      - remote_port: 80
      - remote_port: 443
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadFromPath(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing port name")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadFromPath("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_MissingRemotePort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  mysql:
    remote_host: 127.0.0.1
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadFromPath(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing remote_port")
	}
}

func TestLoadConfig_UnknownFieldTypo(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  databasus:
    remote_port: 4005
    localport: 4005
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadFromPath(cfgPath)
	if err == nil {
		t.Fatal("expected error for unknown field 'localport'")
	}
	if !strings.Contains(err.Error(), "localport") {
		t.Errorf("error should mention 'localport', got: %v", err)
	}
	if !strings.Contains(err.Error(), "local_port") {
		t.Errorf("error should suggest 'local_port', got: %v", err)
	}
}

func TestLoadConfig_UnknownFieldInPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `services:
  gitea:
    ports:
      - name: web
        remote_port: 3000
        localport: 3000
      - name: ssh
        remote_port: 2222
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadFromPath(cfgPath)
	if err == nil {
		t.Fatal("expected error for unknown field 'localport' in port")
	}
	if !strings.Contains(err.Error(), "local_port") {
		t.Errorf("error should suggest 'local_port', got: %v", err)
	}
}

func TestLoad_UnknownTopLevelKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("service:\n  mysql:\n    remote_port: 3306\n"), 0644)

	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("expected error for unknown top-level key 'service'")
	}
	if !strings.Contains(err.Error(), "unknown top-level field") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_ValidTopLevelStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("services:\n  mysql:\n    remote_port: 3306\n"), 0644)

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("valid config must load: %v", err)
	}
	if _, err := cfg.GetService("mysql"); err != nil {
		t.Errorf("mysql should exist: %v", err)
	}
}

func TestLoadConfig_MissingFileIsErrNotExist(t *testing.T) {
	// cmd.loadConfig relies on errors.Is to tell "no config yet" apart from
	// "config is broken"; keep the wrapping intact.
	_, err := LoadFromPath(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing config must wrap os.ErrNotExist, got: %v", err)
	}
}

func TestLoadConfig_PortRange(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "local_port above range",
			yaml:    "services:\n  mysql:\n    remote_port: 3306\n    local_port: 99999\n",
			wantErr: "local_port: 99999 is out of range",
		},
		{
			name:    "local_port negative",
			yaml:    "services:\n  mysql:\n    remote_port: 3306\n    local_port: -1\n",
			wantErr: "local_port: -1 is out of range",
		},
		{
			name:    "remote_port above range",
			yaml:    "services:\n  mysql:\n    remote_port: 70000\n",
			wantErr: "remote_port: 70000 is out of range",
		},
		{
			name:    "multi-port remote_port above range",
			yaml:    "services:\n  gitea:\n    ports:\n      - name: web\n        remote_port: 70000\n      - name: ssh\n        remote_port: 2222\n",
			wantErr: `port "web": remote_port: 70000 is out of range`,
		},
		{
			name:    "multi-port local_port above range",
			yaml:    "services:\n  gitea:\n    ports:\n      - name: web\n        remote_port: 3000\n        local_port: 65536\n      - name: ssh\n        remote_port: 2222\n",
			wantErr: "local_port: 65536 is out of range",
		},
		{
			name:    "boundary ports are accepted",
			yaml:    "services:\n  edge:\n    remote_port: 65535\n    local_port: 1\n",
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := LoadFromPath(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("got error %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("got nil error, want one containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_UpdateCheck(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{
			name: "absent key leaves the check enabled",
			yaml: "services:\n  mysql:\n    remote_port: 3306\n",
			want: true,
		},
		{
			name: "explicitly disabled",
			yaml: "update_check: false\nservices:\n  mysql:\n    remote_port: 3306\n",
			want: false,
		},
		{
			name: "explicitly enabled",
			yaml: "update_check: true\nservices:\n  mysql:\n    remote_port: 3306\n",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := LoadFromPath(path)
			if err != nil {
				t.Fatalf("load failed: %v", err)
			}
			if cfg.UpdateCheck != tt.want {
				t.Errorf("got UpdateCheck = %v, want %v", cfg.UpdateCheck, tt.want)
			}
		})
	}
}

func TestLoadConfig_UpdateCheckTypoIsSuggested(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("updatecheck: false\nservices:\n  mysql:\n    remote_port: 3306\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("got nil error for the misspelled key 'updatecheck', want one")
	}
	if !strings.Contains(err.Error(), `did you mean "update_check"`) {
		t.Errorf("got error %q, want it to suggest update_check", err)
	}
}
