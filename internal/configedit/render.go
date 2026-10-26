package configedit

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Render returns the YAML lines (without line endings) for one service.
// base is the indentation of the service key line, step one nesting level.
// A single port is written in the short form, several under ports:.
func Render(s Service, base, step string) []string {
	lines := []string{base + scalar(s.Name) + ":"}
	child := base + step

	if len(s.Ports) == 1 {
		p := s.Ports[0]
		host := p.RemoteHost
		if host == "" {
			host = s.RemoteHost
		}
		if host != "" {
			lines = append(lines, child+"remote_host: "+scalar(host))
		}
		return append(lines, portKeys(p, child, false)...)
	}

	if s.RemoteHost != "" {
		lines = append(lines, child+"remote_host: "+scalar(s.RemoteHost))
	}
	lines = append(lines, child+"ports:")
	item := child + step
	for _, p := range s.Ports {
		lines = append(lines, item+"- name: "+scalar(p.Name))
		// Keys align under "name", two columns past the dash indent.
		keys := portKeys(p, item+"  ", true)
		lines = append(lines, keys...)
	}
	return lines
}

// Rendered returns the lines Add or Replace write for s: Render at the
// document's own indent and step, so a preview shows exactly what is saved.
func (d *Document) Rendered(s Service) []string {
	return Render(s, d.base, d.step)
}

// scalar returns v as a YAML scalar: plain when it is made of safe characters,
// does not start with '-' and YAML reads it back as the same string;
// double-quoted otherwise (legacy values such as "a: b", "null" or "w #1").
// strconv.Quote's escapes are all valid in YAML double-quoted scalars.
func scalar(v string) string {
	if safePattern.MatchString(v) && !strings.HasPrefix(v, "-") && readsAsString(v) {
		return v
	}
	return strconv.Quote(v)
}

// readsAsString reports whether the plain scalar v resolves to the string v.
func readsAsString(v string) bool {
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(v), &n); err != nil || len(n.Content) != 1 {
		return false
	}
	c := n.Content[0]
	return c.Kind == yaml.ScalarNode && c.Tag == "!!str" && c.Value == v
}

// portKeys renders remote_host (per-port, only when withHost), remote_port
// and local_port lines at the given indent.
func portKeys(p Port, indent string, withHost bool) []string {
	var lines []string
	if withHost && p.RemoteHost != "" {
		lines = append(lines, indent+"remote_host: "+scalar(p.RemoteHost))
	}
	lines = append(lines, indent+"remote_port: "+strconv.Itoa(p.RemotePort))
	switch p.Local {
	case LocalAuto:
		raw := p.AutoRaw
		if raw == "" {
			raw = "auto"
		}
		lines = append(lines, indent+"local_port: "+raw)
	case LocalCustom:
		lines = append(lines, indent+"local_port: "+strconv.Itoa(p.LocalPort))
	}
	return lines
}
