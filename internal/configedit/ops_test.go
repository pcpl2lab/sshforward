package configedit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/config"
)

// load writes out to a temp file and loads it the way sshforward does.
func load(t *testing.T, out []byte) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v\n--- file ---\n%s", err, out)
	}
	return cfg
}

// hasServices checks that cfg holds exactly the named services.
func hasServices(t *testing.T, cfg *config.Config, names ...string) {
	t.Helper()
	if len(cfg.Services) != len(names) {
		t.Errorf("loaded %d services, want %d (%q)", len(cfg.Services), len(names), names)
	}
	for _, n := range names {
		if _, ok := cfg.Services[n]; !ok {
			t.Errorf("service %q not loaded", n)
		}
	}
}

func block(lines []string, eol string) string {
	return strings.Join(lines, eol) + eol
}

var mysql = Service{Name: "mysql", Ports: []Port{{RemotePort: 3306}}}

func TestAdd_Append(t *testing.T) {
	head := "# my tunnels\n" +
		"services:\n" +
		"  # web stuff\n" +
		"  web:\n" +
		"    remote_port: 80 # http\n" +
		"\n" +
		"  # git\n" +
		"  gitea:\n" +
		"    ports:\n" +
		"      - name: web\n" +
		"        remote_port: 3000\n" +
		"      - name: ssh\n" +
		"        remote_port: 2222\n"
	src := head + "\n# end of file\n"
	out := mustParse(t, src).Add(mysql)
	want := head + block(Render(mysql, "  ", "  "), "\n") + "\n# end of file\n"
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	hasServices(t, load(t, out), "web", "gitea", "mysql")
}

func TestAdd_Template(t *testing.T) {
	out := string(mustParse(t, templateText).Add(mysql))
	if !strings.HasPrefix(out, templateText) {
		t.Errorf("template text not kept:\n%s", out)
	}
	if want := templateText + "\n  mysql:\n    remote_port: 3306\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	hasServices(t, load(t, []byte(out)), "mysql")
}

func TestAdd_NoTrailingNewline(t *testing.T) {
	src := "services:\n  web:\n    remote_port: 80"
	out := string(mustParse(t, src).Add(mysql))
	if want := src + "\n  mysql:\n    remote_port: 3306\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	hasServices(t, load(t, []byte(out)), "web", "mysql")
}

func TestAdd_BeforeTopLevelKey(t *testing.T) {
	src := "services:\n  web:\n    remote_port: 80\n# settings\nupdate_check: false\n"
	out := string(mustParse(t, src).Add(mysql))
	want := "services:\n  web:\n    remote_port: 80\n  mysql:\n    remote_port: 3306\n# settings\nupdate_check: false\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if !strings.HasSuffix(out, "update_check: false\n") {
		t.Error("update_check is no longer last")
	}
	cfg := load(t, []byte(out))
	hasServices(t, cfg, "web", "mysql")
	if cfg.UpdateCheck {
		t.Error("UpdateCheck = true, want false")
	}
}

func TestAdd_NoServicesKey(t *testing.T) {
	for _, src := range []string{"update_check: false\n", "update_check: false"} {
		out := string(mustParse(t, src).Add(mysql))
		want := "update_check: false\nservices:\n  mysql:\n    remote_port: 3306\n"
		if out != want {
			t.Errorf("Add on %q = %q, want %q", src, out, want)
		}
		cfg := load(t, []byte(out))
		hasServices(t, cfg, "mysql")
		if cfg.UpdateCheck {
			t.Error("UpdateCheck = true, want false")
		}
	}
}

const threeServices = "services:\n" +
	"  # first\n" +
	"  a:\n" +
	"    remote_port: 1 # one\n" +
	"\n" +
	"  # head of b\n" +
	"  b:\n" +
	"    remote_port: 2\n" +
	"    # inside b\n" +
	"    local_port: 20 # twenty\n" +
	"  # after b\n" +
	"\n" +
	"  # last\n" +
	"  c:\n" +
	"    remote_port: 3\n"

func TestReplace_OnlySpanChanges(t *testing.T) {
	d := mustParse(t, threeServices)
	b := Service{Name: "b", RemoteHost: "db.local", Ports: []Port{{RemotePort: 5432, Local: LocalAuto}}}
	out, err := d.Replace("b", b)
	if err != nil {
		t.Fatal(err)
	}
	before, after, _ := strings.Cut(threeServices, "  b:\n    remote_port: 2\n    # inside b\n    local_port: 20 # twenty\n")
	want := before + block(Render(b, "  ", "  "), "\n") + after
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	cfg := load(t, out)
	hasServices(t, cfg, "a", "b", "c")
	if got := cfg.Services["b"].Ports[0]; got.RemoteHost != "db.local" || got.RemotePort != 5432 || got.LocalPort != 0 {
		t.Errorf("b = %+v", got)
	}
}

func TestReplace_ShapeChange(t *testing.T) {
	two := Service{Name: "a", Ports: []Port{{Name: "web", RemotePort: 80}, {Name: "ssh", RemotePort: 22}}}
	src := "services:\n  a:\n    remote_port: 1\nupdate_check: false\n"
	out, err := mustParse(t, src).Replace("a", two)
	if err != nil {
		t.Fatal(err)
	}
	want := "services:\n  a:\n    ports:\n      - name: web\n        remote_port: 80\n" +
		"      - name: ssh\n        remote_port: 22\nupdate_check: false\n"
	if string(out) != want {
		t.Errorf("short -> ports:\ngot:\n%s\nwant:\n%s", out, want)
	}
	if got := load(t, out).Services["a"]; len(got.Ports) != 2 {
		t.Errorf("loaded %d ports, want 2", len(got.Ports))
	}

	one := Service{Name: "a", Ports: []Port{{Name: "web", RemotePort: 80}}}
	out, err = mustParse(t, want).Replace("a", one)
	if err != nil {
		t.Fatal(err)
	}
	if want := "services:\n  a:\n    remote_port: 80\nupdate_check: false\n"; string(out) != want {
		t.Errorf("ports -> short:\ngot:\n%s\nwant:\n%s", out, want)
	}
	if got := load(t, out).Services["a"]; len(got.Ports) != 1 || got.Ports[0].RemotePort != 80 {
		t.Errorf("loaded %+v", got)
	}
}

// A multi-line value in a service's last field ends past the line its node
// starts on; Replace and Delete must take all of it.
func TestOps_MultiLineLastField(t *testing.T) {
	cases := map[string]string{
		"flow list": "  b:\n" +
			"    ports: [\n" +
			"      {name: web, remote_port: 80},\n" +
			"\n" +
			"      # a comment inside the list\n" +
			"      {name: ssh, remote_port: 22}\n" +
			"    ]\n",
		"folded scalar": "  b:\n" +
			"    remote_port: 2\n" +
			"    remote_host: >-\n" +
			"      db.\n" +
			"\n" +
			"      local\n",
	}
	for name, span := range cases {
		t.Run(name, func(t *testing.T) {
			before := "services:\n  a:\n    remote_port: 1\n"
			after := "  # trailing\n\n  c:\n    remote_port: 3\n"
			d := mustParse(t, before+span+after)
			load(t, []byte(before+span+after))

			out, err := d.Replace("b", Service{Name: "b", Ports: []Port{{RemotePort: 9}}})
			if err != nil {
				t.Fatal(err)
			}
			if want := before + "  b:\n    remote_port: 9\n" + after; string(out) != want {
				t.Errorf("Replace:\ngot:\n%s\nwant:\n%s", out, want)
			}
			hasServices(t, load(t, out), "a", "b", "c")

			out, removed, err := d.Delete("b")
			if err != nil {
				t.Fatal(err)
			}
			if want := before + after; string(out) != want {
				t.Errorf("Delete:\ngot:\n%s\nwant:\n%s", out, want)
			}
			if want := strings.Split(strings.TrimSuffix(span, "\n"), "\n"); !reflect.DeepEqual(removed, want) {
				t.Errorf("removed = %q, want %q", removed, want)
			}
			hasServices(t, load(t, out), "a", "c")
		})
	}
}

func TestRename_KeepsLine(t *testing.T) {
	src := "services:\n  mysql: # prod db\n    remote_port: 3306\n  web:\n    remote_port: 80\n"
	out, err := mustParse(t, src).Rename("mysql", "maria")
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(src, "  mysql: # prod db\n", "  maria: # prod db\n", 1); string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
	hasServices(t, load(t, out), "maria", "web")
}

func TestRename_QuotedKey(t *testing.T) {
	for _, key := range []string{`"my svc"`, `'my svc'`, `"my \" svc"`, `'it''s'`} {
		src := "services:\n  " + key + ":   # c\n    remote_port: 80\n"
		d := mustParse(t, src)
		out, err := d.Rename(d.Names()[0], "mysvc")
		if err != nil {
			t.Fatal(err)
		}
		if want := "services:\n  mysvc:   # c\n    remote_port: 80\n"; string(out) != want {
			t.Errorf("rename %s: got %q, want %q", key, out, want)
		}
		hasServices(t, load(t, out), "mysvc")
	}
}

func TestRename_Existing(t *testing.T) {
	d := mustParse(t, "services:\n  a:\n    remote_port: 1\n  b:\n    remote_port: 2\n")
	if _, err := d.Rename("a", "b"); err == nil {
		t.Error("Rename onto an existing name: expected error")
	}
}

func TestDelete_WithHeadComment(t *testing.T) {
	out, removed, err := mustParse(t, threeServices).Delete("b")
	if err != nil {
		t.Fatal(err)
	}
	want := "services:\n  # first\n  a:\n    remote_port: 1 # one\n\n" +
		"  # after b\n\n  # last\n  c:\n    remote_port: 3\n"
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	wantRemoved := []string{"  # head of b", "  b:", "    remote_port: 2", "    # inside b", "    local_port: 20 # twenty"}
	if !reflect.DeepEqual(removed, wantRemoved) {
		t.Errorf("removed = %q, want %q", removed, wantRemoved)
	}
	hasServices(t, load(t, out), "a", "c")

	// The first service's head comment stops at the services key.
	out, removed, err = mustParse(t, threeServices).Delete("a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "services:\n\n  # head of b\n") {
		t.Errorf("got:\n%s", out)
	}
	if want := []string{"  # first", "  a:", "    remote_port: 1 # one"}; !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %q, want %q", removed, want)
	}
	hasServices(t, load(t, out), "b", "c")
}

func TestDelete_KeepsSeparatedComment(t *testing.T) {
	src := "services:\n  a:\n    remote_port: 1\n  # about b\n\n  b:\n    remote_port: 2\n"
	out, removed, err := mustParse(t, src).Delete("b")
	if err != nil {
		t.Fatal(err)
	}
	if want := "services:\n  a:\n    remote_port: 1\n  # about b\n\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if want := []string{"  b:", "    remote_port: 2"}; !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %q, want %q", removed, want)
	}
	hasServices(t, load(t, out), "a")
}

func TestDroppedComments(t *testing.T) {
	d := mustParse(t, threeServices)
	if got, want := d.DroppedComments("b"), []string{"# inside b", "# twenty"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DroppedComments(b) = %q, want %q", got, want)
	}
	if got, want := d.DroppedComments("a"), []string{"# one"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DroppedComments(a) = %q, want %q", got, want)
	}
	if got := d.DroppedComments("c"); got != nil {
		t.Errorf("DroppedComments(c) = %q, want nil", got)
	}

	d = mustParse(t, "services:\n  mysql: # prod db\n    remote_port: 3306\n")
	if got, want := d.DroppedComments("mysql"), []string{"# prod db"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DroppedComments(mysql) = %q, want %q", got, want)
	}
}

func TestOps_CRLF(t *testing.T) {
	src := strings.ReplaceAll(threeServices, "\n", "\r\n")
	d := mustParse(t, src)
	b := Service{Name: "b", Ports: []Port{{Name: "web", RemotePort: 80}, {Name: "ssh", RemotePort: 22}}}

	replaced, err := d.Replace("b", b)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := d.Rename("b", "bee")
	if err != nil {
		t.Fatal(err)
	}
	deleted, _, err := d.Delete("b")
	if err != nil {
		t.Fatal(err)
	}
	tmpl := mustParse(t, strings.ReplaceAll(templateText, "\n", "\r\n")).Add(mysql)

	for name, out := range map[string][]byte{
		"add": d.Add(mysql), "add template": tmpl,
		"replace": replaced, "rename": renamed, "delete": deleted,
	} {
		text := string(out)
		if !strings.HasSuffix(text, "\r\n") {
			t.Errorf("%s: no final CRLF", name)
		}
		for i, line := range strings.SplitAfter(text, "\n") {
			if line != "" && !strings.HasSuffix(line, "\r\n") {
				t.Errorf("%s: line %d %q does not end in CRLF", name, i+1, line)
			}
		}
		load(t, out)
	}
}

func TestOps_UnknownService(t *testing.T) {
	d := mustParse(t, threeServices)
	if _, err := d.Replace("x", mysql); err == nil {
		t.Error("Replace: expected error")
	}
	if _, err := d.Rename("x", "y"); err == nil {
		t.Error("Rename: expected error")
	}
	if _, _, err := d.Delete("x"); err == nil {
		t.Error("Delete: expected error")
	}
}

// The source is not modified and the Document stays usable.
func TestOps_DocumentUnchanged(t *testing.T) {
	d := mustParse(t, threeServices)
	d.Add(mysql)
	if _, _, err := d.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(d.lines, ""); got != threeServices {
		t.Errorf("lines changed:\n%s", got)
	}
}

func TestAdd_ExplicitNullServices(t *testing.T) {
	tests := map[string]string{
		"services: ~\n":        "services:\n",
		"services: null\n":     "services:\n",
		"services: NULL\n":     "services:\n",
		"services: ~ # note\n": "services: # note\n",
		"services:   Null  \n": "services:  \n",
	}
	for src, head := range tests {
		out := string(mustParse(t, src+"update_check: false\n").Add(mysql))
		if want := head + "  mysql:\n    remote_port: 3306\nupdate_check: false\n"; out != want {
			t.Errorf("Add on %q = %q, want %q", src, out, want)
		}
		cfg := load(t, []byte(out))
		hasServices(t, cfg, "mysql")
		if cfg.UpdateCheck {
			t.Errorf("%q: UpdateCheck = true, want false", src)
		}
	}
}

func TestReplace_NameCollision(t *testing.T) {
	d := mustParse(t, threeServices)
	if _, err := d.Replace("a", Service{Name: "b", Ports: []Port{{RemotePort: 1}}}); err == nil {
		t.Error("Replace renaming onto another service: expected error")
	}
	if _, err := d.Replace("a", Service{Name: "z", Ports: []Port{{RemotePort: 1}}}); err != nil {
		t.Errorf("Replace with a new name: %v", err)
	}
}

// Legacy values that are valid only quoted must keep their meaning when the
// service is edited or renamed.
var legacyQuoted = map[string]string{
	"key colon": "services:\n  \"a: b\":\n    remote_port: 1\n",
	"key null":  "services:\n  \"null\":\n    remote_port: 1\n",
	"key dash":  "services:\n  \"-\":\n    remote_port: 1\n",
	"port name hash": "services:\n  g:\n    ports:\n      - name: \"w #1\"\n        remote_port: 1\n" +
		"      - name: s\n        remote_port: 2\n",
	"host colon": "services:\n  h:\n    remote_host: \"x: y\"\n    remote_port: 1\n",
	"port host colon": "services:\n  g:\n    ports:\n      - name: w\n        remote_host: \"x: y\"\n        remote_port: 1\n" +
		"      - name: s\n        remote_port: 2\n",
}

func TestReplace_LegacyQuotedValues(t *testing.T) {
	for label, src := range legacyQuoted {
		t.Run(label, func(t *testing.T) {
			d := mustParse(t, src)
			name := d.Names()[0]
			s, err := d.Service(name)
			if err != nil {
				t.Fatal(err)
			}
			s.Ports[0].RemotePort = 99
			out, err := d.Replace(name, s)
			if err != nil {
				t.Fatal(err)
			}
			if diff := describeDiff(load(t, out), Expected(load(t, []byte(src)), name, &s)); diff != "" {
				t.Errorf("%s\n--- out ---\n%s", diff, out)
			}
		})
	}
}

func TestRename_QuotesWhenNeeded(t *testing.T) {
	// The form's validator rejects these names; Rename itself still keeps
	// the meaning of whatever it is given.
	for _, newName := range []string{"c: d", "null", "-", "w #1"} {
		if ValidateName(newName) == nil {
			t.Errorf("ValidateName(%q) = nil, want error", newName)
		}
		src := legacyQuoted["key colon"]
		out, err := mustParse(t, src).Rename("a: b", newName)
		if err != nil {
			t.Fatal(err)
		}
		before := load(t, []byte(src))
		if diff := describeDiff(load(t, out), ExpectedRename(before, "a: b", newName)); diff != "" {
			t.Errorf("rename to %q: %s\n%s", newName, diff, out)
		}
	}
	// Renaming a legacy quoted key to a plain name keeps the rest of the line.
	out, err := mustParse(t, "services:\n  \"null\": # c\n    remote_port: 1\n").Rename("null", "n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "services:\n  n: # c\n    remote_port: 1\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestDelete_KeepsPreviousServiceIndentedComment(t *testing.T) {
	src := "services:\n  mysql:\n    remote_port: 3306\n    # local_port: 13306\n  # about gitea\n  gitea:\n    remote_port: 3000\n"
	out, removed, err := mustParse(t, src).Delete("gitea")
	if err != nil {
		t.Fatal(err)
	}
	if want := "services:\n  mysql:\n    remote_port: 3306\n    # local_port: 13306\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if want := []string{"  # about gitea", "  gitea:", "    remote_port: 3000"}; !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %q, want %q", removed, want)
	}
	hasServices(t, load(t, out), "mysql")
}

func TestEdit_NullPrefill(t *testing.T) {
	// remote_host: ~ loads as 127.0.0.1; an edit must keep that meaning.
	src := "services:\n  h:\n    remote_host: ~\n    remote_port: 1\n"
	d := mustParse(t, src)
	s, err := d.Service("h")
	if err != nil {
		t.Fatal(err)
	}
	s.Ports[0].RemotePort = 2
	out, err := d.Replace("h", s)
	if err != nil {
		t.Fatal(err)
	}
	if diff := describeDiff(load(t, out), Expected(load(t, []byte(src)), "h", &s)); diff != "" {
		t.Errorf("%s\n%s", diff, out)
	}
}
