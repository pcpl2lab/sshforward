package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeAged creates name in dir with the given content and modification time.
func writeAged(t *testing.T, dir, name, content string, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCleanOldLogs(t *testing.T) {
	dir := t.TempDir()
	expired := time.Now().Add(-LogRetention - time.Hour)
	recent := time.Now().Add(-LogRetention + time.Hour)

	files := map[string]bool{ // path -> should survive
		writeAged(t, dir, "p1-gitea-web.json", "{}", expired):               true,
		writeAged(t, dir, "p1-gitea-web.log", "running", expired):           true, // has a state file
		writeAged(t, dir, "p5-coolify.log", "failed start", expired):        false,
		writeAged(t, dir, "p5-coolify.20260101-120000.log", "old", expired): false,
		writeAged(t, dir, "p9-mysql.log", "failed start", recent):           true,
		writeAged(t, dir, "notes.txt", "not ours", expired):                 true,
	}

	removed, err := CleanOldLogs(dir)
	if err != nil {
		t.Fatalf("CleanOldLogs: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d logs, want 2", removed)
	}
	for path, want := range files {
		if got := exists(path); got != want {
			t.Errorf("%s exists=%v, want %v", filepath.Base(path), got, want)
		}
	}
}

func TestCleanOldLogs_MissingDir(t *testing.T) {
	removed, err := CleanOldLogs(filepath.Join(t.TempDir(), "absent"))
	if err != nil || removed != 0 {
		t.Errorf("got (%d, %v), want (0, nil)", removed, err)
	}
}

func TestArchiveLog(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Date(2026, 10, 21, 16, 57, 53, 0, time.Local)
	path := writeAged(t, dir, "p1-gitea-web.log", "Connection reset", mtime)

	if err := archiveLog(path); err != nil {
		t.Fatalf("archiveLog: %v", err)
	}
	if exists(path) {
		t.Error("log still in place; the next run would append to it")
	}
	data, err := os.ReadFile(filepath.Join(dir, "p1-gitea-web.20261021-165753.log"))
	if err != nil || string(data) != "Connection reset" {
		t.Errorf("archived log = %q, %v; want the previous run's content", data, err)
	}
}

func TestArchiveLog_EmptyIsDropped(t *testing.T) {
	dir := t.TempDir()
	path := writeAged(t, dir, "p1-gitea-web.log", "", time.Now())

	if err := archiveLog(path); err != nil {
		t.Fatalf("archiveLog: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("expected nothing left, got %d entries", len(entries))
	}
}

func TestArchiveLog_Missing(t *testing.T) {
	if err := archiveLog(filepath.Join(t.TempDir(), "none.log")); err != nil {
		t.Errorf("archiveLog on a missing log: %v", err)
	}
}

func TestListArchivedLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeAged(t, dir, "p1-gitea-web.json", "{}", now)
	writeAged(t, dir, "p1-gitea-web.log", "running", now) // current run: not archived
	writeAged(t, dir, "p1-gitea-web.20261020-100000.log", "web old", now.Add(-2*time.Hour))
	writeAged(t, dir, "p1-gitea-ssh.log", "ssh failed", now.Add(-time.Hour))
	writeAged(t, dir, "p1-gitea2.log", "other service", now)
	writeAged(t, dir, "p5-coolify.log", "other host", now)

	all, err := ListArchivedLogs(dir, "")
	if err != nil {
		t.Fatalf("ListArchivedLogs: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("got %d archived logs, want 4", len(all))
	}

	gitea, err := ListArchivedLogs(dir, StateKey("p1", "gitea", ""))
	if err != nil {
		t.Fatalf("ListArchivedLogs: %v", err)
	}
	var names []string
	for _, l := range gitea {
		names = append(names, filepath.Base(l.Path))
	}
	want := "p1-gitea-web.20261020-100000.log,p1-gitea-ssh.log" // oldest first
	if got := strings.Join(names, ","); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got := gitea[1].Read(); got != "ssh failed" {
		t.Errorf("Read() = %q", got)
	}
}
