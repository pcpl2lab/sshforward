package configedit

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/config"
)

const twoServices = "services:\n  mysql:\n    remote_port: 3306\n  redis:\n    remote_port: 6379\n"

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func noTempFiles(t *testing.T, dir string) {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, ".config-*"))
	if len(m) != 0 {
		t.Errorf("temp files left behind: %v", m)
	}
}

func simple(name string, port int) Service {
	return Service{Name: name, Ports: []Port{{RemotePort: port}}}
}

func TestExpected(t *testing.T) {
	before := load(t, []byte(twoServices))
	snapshot := map[string]config.Service{}
	for k, v := range before.Services {
		snapshot[k] = v
	}
	pg := simple("pg", 5432)
	mysql2 := simple("mysql", 3307)
	renamed := simple("db", 3306)

	tests := []struct {
		name    string
		oldName string
		s       *Service
		want    map[string]config.Service
	}{
		{"add", "", &pg, map[string]config.Service{
			"mysql": before.Services["mysql"], "redis": before.Services["redis"], "pg": pg.Resolved()}},
		{"edit", "mysql", &mysql2, map[string]config.Service{
			"mysql": mysql2.Resolved(), "redis": before.Services["redis"]}},
		{"rename", "mysql", &renamed, map[string]config.Service{
			"db": renamed.Resolved(), "redis": before.Services["redis"]}},
		{"delete", "mysql", nil, map[string]config.Service{
			"redis": before.Services["redis"]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Expected(before, tt.oldName, tt.s)
			if !reflect.DeepEqual(got.Services, tt.want) {
				t.Errorf("Services = %v, want %v", got.Services, tt.want)
			}
			if got.UpdateCheck != before.UpdateCheck {
				t.Errorf("UpdateCheck = %v, want %v", got.UpdateCheck, before.UpdateCheck)
			}
			if !reflect.DeepEqual(before.Services, snapshot) {
				t.Errorf("before was mutated: %v", before.Services)
			}
		})
	}
}

func TestCommit_Writes(t *testing.T) {
	path := writeConfig(t, twoServices)
	updated := twoServices + "  pg:\n    remote_port: 5432\n"
	pg := simple("pg", 5432)
	want := Expected(load(t, []byte(twoServices)), "", &pg)

	if err := Commit(path, []byte(twoServices), []byte(updated), want); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != updated {
		t.Errorf("content = %q, want %q", got, updated)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
		}
	}
	noTempFiles(t, filepath.Dir(path))
}

func TestCommit_ChangedOnDisk(t *testing.T) {
	path := writeConfig(t, twoServices)
	changed := twoServices + "# edited elsewhere\n"
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	want := load(t, []byte(twoServices))
	err := Commit(path, []byte(twoServices), []byte(twoServices), want)
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
	if got := readFile(t, path); got != changed {
		t.Errorf("file was modified: %q", got)
	}
	noTempFiles(t, filepath.Dir(path))
}

func TestCommit_InvalidResult(t *testing.T) {
	path := writeConfig(t, twoServices)
	want := load(t, []byte(twoServices))
	err := Commit(path, []byte(twoServices), []byte("services:\n  mysql:\n    remote_port: [\n"), want)
	if err == nil {
		t.Fatal("want an error for an updated file that does not load")
	}
	if got := readFile(t, path); got != twoServices {
		t.Errorf("file was modified: %q", got)
	}
	noTempFiles(t, filepath.Dir(path))
}

func TestCommit_UnexpectedMeaning(t *testing.T) {
	path := writeConfig(t, twoServices)
	pg := simple("pg", 5432)
	want := Expected(load(t, []byte(twoServices)), "", &pg)
	// Loads fine, but the port differs from what the preview promised.
	updated := twoServices + "  pg:\n    remote_port: 5433\n"
	err := Commit(path, []byte(twoServices), []byte(updated), want)
	if err == nil {
		t.Fatal("want an error for a result that differs from the preview")
	}
	if !strings.Contains(err.Error(), `"pg"`) || !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("error should name the service and say nothing was written: %v", err)
	}
	if got := readFile(t, path); got != twoServices {
		t.Errorf("file was modified: %q", got)
	}
	noTempFiles(t, filepath.Dir(path))
}

func TestCommit_RefusesSilentMisplacement(t *testing.T) {
	path := writeConfig(t, twoServices)
	pg := simple("pg", 5432)
	want := Expected(load(t, []byte(twoServices)), "", &pg)
	// The new service text ended up inside mysql's quoted value: valid YAML,
	// every existing service intact, but pg does not exist.
	updated := "services:\n  mysql:\n    remote_port: 3306\n    remote_host: \"a\n  pg:\n    remote_port: 5432\"\n  redis:\n    remote_port: 6379\n"
	err := Commit(path, []byte(twoServices), []byte(updated), want)
	if err == nil {
		t.Fatal("want an error for a misplaced service")
	}
	if !strings.Contains(err.Error(), `service "pg" missing`) {
		t.Errorf("error should name the missing service: %v", err)
	}
	if got := readFile(t, path); got != twoServices {
		t.Errorf("file was modified: %q", got)
	}
	noTempFiles(t, filepath.Dir(path))
}

func TestCommit_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	if err := os.WriteFile(target, []byte(twoServices), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	updated := twoServices + "  pg:\n    remote_port: 5432\n"
	pg := simple("pg", 5432)
	want := Expected(load(t, []byte(twoServices)), "", &pg)

	if err := Commit(link, []byte(twoServices), []byte(updated), want); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("config is no longer a symlink")
	}
	if got := readFile(t, target); got != updated {
		t.Errorf("target = %q, want %q", got, updated)
	}
}

func TestCommit_NewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "services:\n  pg:\n    remote_port: 5432\n"
	pg := simple("pg", 5432)
	want := Expected(&config.Config{Services: map[string]config.Service{}, UpdateCheck: true}, "", &pg)

	if err := Commit(path, nil, []byte(content), want); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestExpectedRename(t *testing.T) {
	before := load(t, []byte(twoServices))
	got := ExpectedRename(before, "mysql", "maria")
	if _, ok := before.Services["maria"]; ok {
		t.Error("before was modified")
	}
	if _, ok := got.Services["mysql"]; ok {
		t.Error("old name still present")
	}
	if !reflect.DeepEqual(got.Services["maria"], before.Services["mysql"]) {
		t.Errorf("renamed service = %+v, want %+v", got.Services["maria"], before.Services["mysql"])
	}
	if !reflect.DeepEqual(got.Services["redis"], before.Services["redis"]) || got.UpdateCheck != before.UpdateCheck {
		t.Error("other fields changed")
	}
}

func TestCommit_RenameNullHost(t *testing.T) {
	// The loaded config, not the prefill, says what a renamed service means.
	src := "services:\n  h:\n    remote_host: ~\n    remote_port: 1\n"
	path := writeConfig(t, src)
	out, err := mustParse(t, src).Rename("h", "g")
	if err != nil {
		t.Fatal(err)
	}
	if err := Commit(path, []byte(src), out, ExpectedRename(load(t, []byte(src)), "h", "g")); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
