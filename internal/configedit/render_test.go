package configedit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/config"
)

var (
	shortSvc    = Service{Name: "mysql", Ports: []Port{{RemotePort: 3306}}}
	shortAllSvc = Service{Name: "db", RemoteHost: "10.0.0.5", Ports: []Port{{RemotePort: 3306, Local: LocalCustom, LocalPort: 13306}}}
	multiSvc    = Service{Name: "gitea", RemoteHost: "127.0.0.1", Ports: []Port{
		{Name: "web", RemotePort: 3000},
		{Name: "ssh", RemotePort: 2222, Local: LocalAuto, AutoRaw: "0"},
	}}
	collapseSvc = Service{Name: "c", RemoteHost: "a.example", Ports: []Port{{RemoteHost: "b.example", RemotePort: 80}}}
)

func TestRender_ShortForm(t *testing.T) {
	got := Render(shortSvc, "  ", "  ")
	want := []string{"  mysql:", "    remote_port: 3306"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_ShortFormAll(t *testing.T) {
	got := Render(shortAllSvc, "  ", "  ")
	want := []string{"  db:", "    remote_host: 10.0.0.5", "    remote_port: 3306", "    local_port: 13306"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_Multi(t *testing.T) {
	got := Render(multiSvc, "  ", "  ")
	want := []string{
		"  gitea:",
		"    remote_host: 127.0.0.1",
		"    ports:",
		"      - name: web",
		"        remote_port: 3000",
		"      - name: ssh",
		"        remote_port: 2222",
		"        local_port: 0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_CollapseUsesPortOverride(t *testing.T) {
	got := Render(collapseSvc, "  ", "  ")
	want := []string{"  c:", "    remote_host: b.example", "    remote_port: 80"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_Step4(t *testing.T) {
	got := Render(shortSvc, "  ", "    ")
	want := []string{"  mysql:", "      remote_port: 3306"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	got = Render(multiSvc, "  ", "    ")
	if got[2] != "      ports:" || got[3] != "          - name: web" || got[4] != "            remote_port: 3000" {
		t.Errorf("unexpected multi step-4 output: %q", got)
	}
}

func TestResolved_MatchesLoader(t *testing.T) {
	for _, s := range []Service{shortSvc, shortAllSvc, multiSvc, collapseSvc} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		data := "services:\n" + strings.Join(Render(s, "  ", "  "), "\n") + "\n"
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.LoadFromPath(path)
		if err != nil {
			t.Fatalf("%s: load: %v\n%s", s.Name, err, data)
		}
		if got, want := cfg.Services[s.Name], s.Resolved(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: loader %+v != Resolved %+v\n%s", s.Name, got, want, data)
		}
	}
}

func TestRender_QuotesWhenNeeded(t *testing.T) {
	s := Service{Name: "a: b", RemoteHost: "x: y", Ports: []Port{
		{Name: "w #1", RemoteHost: "-", RemotePort: 1},
		{Name: "null", RemotePort: 2},
		{Name: "123", RemotePort: 3},
	}}
	got := Render(s, "  ", "  ")
	want := []string{
		`  "a: b":`,
		`    remote_host: "x: y"`,
		`    ports:`,
		`      - name: "w #1"`,
		`        remote_host: "-"`,
		`        remote_port: 1`,
		`      - name: "null"`,
		`        remote_port: 2`,
		`      - name: "123"`,
		`        remote_port: 3`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// Safe values stay plain.
	if got := Render(shortAllSvc, "", "  "); got[0] != "db:" || got[1] != "  remote_host: 10.0.0.5" {
		t.Errorf("plain values quoted: %q", got)
	}
}

func TestDocument_Rendered(t *testing.T) {
	// Base and step come from the file, so the preview matches the write.
	src := "services:\n    a:\n        remote_port: 1\n"
	d := mustParse(t, src)
	s := Service{Name: "b", Ports: []Port{{RemotePort: 2}}}
	got := d.Rendered(s)
	want := []string{"    b:", "        remote_port: 2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Rendered = %q, want %q", got, want)
	}
	if out := string(d.Add(s)); !strings.HasSuffix(out, strings.Join(want, "\n")+"\n") {
		t.Errorf("Add wrote %q, preview %q", out, want)
	}
	s.Name = "a"
	out, err := d.Replace("a", s)
	if err != nil {
		t.Fatal(err)
	}
	if want := "services:\n" + strings.Join(d.Rendered(s), "\n") + "\n"; string(out) != want {
		t.Errorf("Replace wrote %q, want %q", out, want)
	}
}
