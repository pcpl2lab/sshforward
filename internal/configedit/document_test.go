package configedit

import (
	"errors"
	"reflect"
	"testing"
)

// templateText is a copy of cmd.defaultConfigTemplate.
const templateText = `services:
  # example:
  # mysql:
  #   remote_port: 3306
  # gitea:
  #   remote_host: 127.0.0.1
  #   ports:
  #     - name: web
  #       remote_port: 3000
  #     - name: ssh
  #       remote_port: 2222
`

func mustParse(t *testing.T, src string) *Document {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

func TestParse_Names(t *testing.T) {
	d := mustParse(t, "services:\n  mysql:\n    remote_port: 3306\n  gitea:\n    remote_port: 3000\n")
	if got, want := d.Names(), []string{"mysql", "gitea"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %q, want %q", got, want)
	}
}

func TestParse_Empty(t *testing.T) {
	d := mustParse(t, "")
	if n := d.Names(); len(n) != 0 {
		t.Errorf("Names() = %q, want empty", n)
	}
	if d.servicesLine != -1 {
		t.Errorf("servicesLine = %d, want -1", d.servicesLine)
	}
}

func TestParse_Template(t *testing.T) {
	d := mustParse(t, templateText)
	if n := d.Names(); len(n) != 0 {
		t.Errorf("Names() = %q, want empty", n)
	}
	if d.servicesLine != 0 || d.servicesCol != 0 {
		t.Errorf("services at %d:%d, want 0:0", d.servicesLine, d.servicesCol)
	}
	if d.base != "  " || d.step != "  " {
		t.Errorf("base %q step %q, want two spaces each", d.base, d.step)
	}
}

func TestParse_NoServicesKey(t *testing.T) {
	d := mustParse(t, "update_check: false\n")
	if n := d.Names(); len(n) != 0 {
		t.Errorf("Names() = %q, want empty", n)
	}
	if d.servicesLine != -1 {
		t.Errorf("servicesLine = %d, want -1", d.servicesLine)
	}
}

func TestService_Prefill(t *testing.T) {
	src := `services:
  plain:
    remote_port: 3306
  auto:
    remote_port: 3306
    local_port: auto
  zero:
    remote_port: 3306
    local_port: 0
  same:
    remote_host: db.local
    remote_port: 3306
    local_port: 3306
  empty:
    remote_port: 22
    local_port: ""
  gitea:
    remote_host: 127.0.0.1
    ports:
      - name: web
        remote_port: 3000
        remote_host: web.local
      - name: ssh
        remote_port: 2222
        local_port: 2223
`
	d := mustParse(t, src)
	tests := []struct {
		name string
		want Service
	}{
		{"plain", Service{Name: "plain", Ports: []Port{{RemotePort: 3306}}}},
		{"auto", Service{Name: "auto", Ports: []Port{{RemotePort: 3306, Local: LocalAuto, AutoRaw: "auto"}}}},
		{"zero", Service{Name: "zero", Ports: []Port{{RemotePort: 3306, Local: LocalAuto, AutoRaw: "0"}}}},
		{"same", Service{Name: "same", RemoteHost: "db.local", Ports: []Port{{RemotePort: 3306, Local: LocalCustom, LocalPort: 3306}}}},
		{"empty", Service{Name: "empty", Ports: []Port{{RemotePort: 22}}}},
		{"gitea", Service{Name: "gitea", RemoteHost: "127.0.0.1", Ports: []Port{
			{Name: "web", RemoteHost: "web.local", RemotePort: 3000},
			{Name: "ssh", RemotePort: 2222, Local: LocalCustom, LocalPort: 2223},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Service(tt.name)
			if err != nil {
				t.Fatalf("Service: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	if _, err := d.Service("missing"); err == nil {
		t.Error("Service(missing): expected error")
	}
}

func TestParse_QuotedKey(t *testing.T) {
	d := mustParse(t, "services:\n  \"my svc\":\n    remote_port: 80\n")
	if got, want := d.Names(), []string{"my svc"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %q, want %q", got, want)
	}
	s, err := d.Service("my svc")
	if err != nil {
		t.Fatalf("Service: %v", err)
	}
	want := Service{Name: "my svc", Ports: []Port{{RemotePort: 80}}}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("got %+v, want %+v", s, want)
	}
}

func TestParse_FlowStyle(t *testing.T) {
	for _, src := range []string{
		"services: {a: {remote_port: 1}}\n",
		"services:\n  a: {remote_port: 1}\n",
	} {
		if _, err := Parse([]byte(src)); !errors.Is(err, ErrFlowStyle) {
			t.Errorf("Parse(%q) error = %v, want ErrFlowStyle", src, err)
		}
	}
}

func TestParse_Span(t *testing.T) {
	src := "services:\n" + // 0
		"  mysql:\n" + // 1
		"    remote_port: 3306 # main db\n" + // 2
		"    # trailing comment\n" + // 3
		"\n" + // 4
		"  gitea:\n" + // 5
		"    ports:\n" + // 6
		"      - name: web\n" + // 7
		"        remote_port: 3000\n" + // 8
		"      - name: ssh\n" + // 9
		"        remote_port: 2222\n" + // 10
		"update_check: false\n" // 11
	d := mustParse(t, src)
	want := []serviceSpan{
		{name: "mysql", keyLine: 1, endLine: 2},
		{name: "gitea", keyLine: 5, endLine: 10},
	}
	if !reflect.DeepEqual(d.spans, want) {
		t.Errorf("spans = %+v, want %+v", d.spans, want)
	}

	// A service with nothing after its key spans only the key line.
	d = mustParse(t, "services:\n  empty:\n\n  next:\n    remote_port: 1\n")
	want = []serviceSpan{
		{name: "empty", keyLine: 1, endLine: 1},
		{name: "next", keyLine: 3, endLine: 4},
	}
	if !reflect.DeepEqual(d.spans, want) {
		t.Errorf("spans = %+v, want %+v", d.spans, want)
	}
}

func TestParse_LayoutAndEOL(t *testing.T) {
	src := "services:\r\n    mysql:\r\n        remote_port: 3306\r\n"
	d := mustParse(t, src)
	if d.eol != "\r\n" {
		t.Errorf("eol = %q, want CRLF", d.eol)
	}
	if d.base != "    " || d.step != "    " {
		t.Errorf("base %q step %q, want four spaces each", d.base, d.step)
	}
	if len(d.lines) != 4 || d.lines[1] != "    mysql:\r\n" {
		t.Errorf("lines = %q", d.lines)
	}

	d = mustParse(t, "services:\n  mysql:\n    remote_port: 3306\n")
	if d.eol != "\n" {
		t.Errorf("eol = %q, want LF", d.eol)
	}
}

func TestService_NullPrefill(t *testing.T) {
	src := "services:\n  a:\n    remote_host: ~\n    remote_port: 1\n" +
		"  b:\n    remote_host: null\n    ports:\n      - name: null\n        remote_host: ~\n        remote_port: 1\n" +
		"      - name: s\n        remote_port: 2\n"
	d := mustParse(t, src)
	a, err := d.Service("a")
	if err != nil {
		t.Fatal(err)
	}
	if a.RemoteHost != "" {
		t.Errorf("a.RemoteHost = %q, want empty", a.RemoteHost)
	}
	b, err := d.Service("b")
	if err != nil {
		t.Fatal(err)
	}
	if b.RemoteHost != "" || b.Ports[0].Name != "" || b.Ports[0].RemoteHost != "" {
		t.Errorf("b = %+v, want null values empty", b)
	}
}
