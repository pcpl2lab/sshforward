package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/pcpl2lab/sshforward/internal/configedit"
)

// setHome points config.DefaultConfigPath at a fresh temp directory.
func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestOpenDocument_CreatesFromTemplate(t *testing.T) {
	home := setHome(t)

	path, src, doc, cfg, err := openDocument(true)
	if err != nil {
		t.Fatalf("openDocument: %v", err)
	}
	want := filepath.Join(home, ".sshforward", "config.yaml")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if string(data) != defaultConfigTemplate {
		t.Errorf("file content = %q, want the template", data)
	}
	if string(src) != defaultConfigTemplate {
		t.Errorf("src = %q, want the template", src)
	}
	if names := doc.Names(); len(names) != 0 {
		t.Errorf("Names() = %v, want none", names)
	}
	if len(cfg.Services) != 0 {
		t.Errorf("cfg.Services = %v, want none", cfg.Services)
	}
}

func TestOpenDocument_InvalidConfig(t *testing.T) {
	home := setHome(t)
	dir := filepath.Join(home, ".sshforward")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// The loader suggests a key only for a missing or extra underscore.
	const bad = "services:\n  a:\n    remoteport: 1\n"
	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, _, _, err := openDocument(true)
	if err == nil || !strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("err = %v, want the loader's 'did you mean' error", err)
	}
	if data, _ := os.ReadFile(file); string(data) != bad {
		t.Errorf("file changed to %q", data)
	}
}

func TestRequireTerminal_NotTTY(t *testing.T) {
	// Under go test stdin and stdout are not a terminal.
	err := requireTerminal()
	if err == nil || !strings.Contains(err.Error(), "sshforward edit") {
		t.Fatalf("err = %v, want an error pointing to 'sshforward edit'", err)
	}
}

func TestOpenDocument_ChangedWhileLoading(t *testing.T) {
	home := setHome(t)
	dir := filepath.Join(home, ".sshforward")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte("services:\n  a:\n    remote_port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The file changes right after the loader read it: cfg and src would
	// describe different files.
	orig := loadConfigFile
	t.Cleanup(func() { loadConfigFile = orig })
	loadConfigFile = func(path string) (*config.Config, error) {
		cfg, err := orig(path)
		if werr := os.WriteFile(path, []byte("services:\n  b:\n    remote_port: 2\n"), 0o600); werr != nil {
			t.Fatal(werr)
		}
		return cfg, err
	}

	_, _, _, _, err := openDocument(false)
	if !errors.Is(err, configedit.ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
}
