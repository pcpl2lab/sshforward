package cmd

import (
	"strings"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/configedit"
	"github.com/pcpl2lab/sshforward/internal/tunnel"
)

func TestLiveTunnelWarnings(t *testing.T) {
	web := &tunnel.TunnelState{Host: "prod", Service: "gitea", PortName: "web"}
	ssh := &tunnel.TunnelState{Host: "prod", Service: "gitea", PortName: "ssh"}
	mysql := &tunnel.TunnelState{Host: "prod", Service: "mysql"}
	alive := func(s *tunnel.TunnelState) bool { return s != ssh }

	got := liveTunnelWarnings([]*tunnel.TunnelState{web, ssh, mysql}, "gitea", alive)
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(got), got)
	}
	for _, want := range []string{"prod/gitea (web)", "sshforward stop prod gitea"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning %q does not contain %q", got[0], want)
		}
	}
}

func TestLiveTunnelWarnings_SinglePort(t *testing.T) {
	mysql := &tunnel.TunnelState{Host: "prod", Service: "mysql"}
	got := liveTunnelWarnings([]*tunnel.TunnelState{mysql}, "mysql", func(*tunnel.TunnelState) bool { return true })
	want := "tunnel prod/mysql is running with the old settings; stop it with: sshforward stop prod mysql"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %q, want [%q]", got, want)
	}
}

func TestChangedLine(t *testing.T) {
	tests := []struct {
		name, a, b, before, after string
	}{
		{"LF", "x:\n  a: 1\n  b: 2\n", "x:\n  c: 1\n  b: 2\n", "  a: 1", "  c: 1"},
		{"CRLF", "x:\r\n  a: 1\r\n", "x:\r\n  c: 1\r\n", "  a: 1", "  c: 1"},
		{"no final newline", "x:\n  a: 1", "x:\n  c: 1", "  a: 1", "  c: 1"},
		{"no difference", "x:\n  a: 1\n", "x:\n  a: 1\n", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := changedLine([]byte(tt.a), []byte(tt.b))
			if before != tt.before || after != tt.after {
				t.Errorf("changedLine = %q, %q; want %q, %q", before, after, tt.before, tt.after)
			}
		})
	}
}

func TestCollapseWarning(t *testing.T) {
	multi := configedit.Service{Name: "g", RemoteHost: "a.example", Ports: []configedit.Port{
		{Name: "web", RemoteHost: "b.example", RemotePort: 80},
		{Name: "ssh", RemotePort: 22},
	}}
	collapsed := multi
	collapsed.Ports = multi.Ports[:1]
	if got := collapseWarning(multi, collapsed); !strings.Contains(got, "remote_host b.example from port web is kept") {
		t.Errorf("collapseWarning = %q, want the kept-host warning", got)
	}

	sameHost := collapsed
	sameHost.Ports = []configedit.Port{{Name: "web", RemoteHost: "a.example", RemotePort: 80}}
	noPortHost := collapsed
	noPortHost.Ports = []configedit.Port{{Name: "web", RemotePort: 80}}
	noSvcHost := collapsed
	noSvcHost.RemoteHost = ""
	for name, s := range map[string]configedit.Service{
		"same host": sameHost, "no port host": noPortHost, "no service host": noSvcHost, "still multi": multi,
	} {
		if got := collapseWarning(multi, s); got != "" {
			t.Errorf("%s: collapseWarning = %q, want none", name, got)
		}
	}
	// A service that was single-port already has nothing to collapse.
	if got := collapseWarning(collapsed, collapsed); got != "" {
		t.Errorf("single to single: collapseWarning = %q, want none", got)
	}
}

func TestSameRendering(t *testing.T) {
	orig := configedit.Service{Name: "mysql", Ports: []configedit.Port{{RemotePort: 3306}}}
	// Add a port, then remove it again: the remaining port keeps the name it
	// was given, which a single-port service does not write.
	s := orig
	s.Ports = []configedit.Port{{Name: "db", RemotePort: 3306}}
	if !sameRendering(orig, s) {
		t.Error("add-then-remove round trip counted as a change")
	}
	s.Ports = []configedit.Port{{RemotePort: 3307}}
	if sameRendering(orig, s) {
		t.Error("changed port counted as unchanged")
	}
}
