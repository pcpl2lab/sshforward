// Package configedit edits the services section of the sshforward config
// while keeping the rest of the file intact.
package configedit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pcpl2lab/sshforward/internal/config"
)

// safePattern restricts names and hosts to the same characters as the
// tunnel package's host validation.
var safePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// LocalMode says how a port's local_port is written.
type LocalMode int

const (
	LocalSame   LocalMode = iota // local_port omitted: same as remote_port
	LocalAuto                    // random free port
	LocalCustom                  // explicit LocalPort
)

// Port is one forwarded port of a service.
type Port struct {
	Name       string // "" only in a single-port service
	RemoteHost string // per-port override as written; "" = none
	RemotePort int
	Local      LocalMode
	LocalPort  int    // used only with LocalCustom
	AutoRaw    string // literal of an existing auto value ("auto" or "0"); "" renders "auto"
}

// Service is an editable service definition.
type Service struct {
	Name       string
	RemoteHost string // service level; "" = omitted
	Ports      []Port // len >= 1
}

// Resolved returns the service as config.LoadFromPath would load it.
func (s Service) Resolved() config.Service {
	multi := len(s.Ports) > 1
	ports := make([]config.PortMapping, 0, len(s.Ports))
	for _, p := range s.Ports {
		host := p.RemoteHost
		if host == "" {
			host = s.RemoteHost
		}
		if host == "" {
			host = "127.0.0.1"
		}
		local := p.RemotePort
		switch p.Local {
		case LocalAuto:
			local = 0
		case LocalCustom:
			local = p.LocalPort
		}
		name := ""
		if multi {
			name = p.Name
		}
		ports = append(ports, config.PortMapping{
			Name:       name,
			RemoteHost: host,
			RemotePort: p.RemotePort,
			LocalPort:  local,
		})
	}
	return config.Service{Ports: ports}
}

// ValidateName checks a service or port name.
func ValidateName(s string) error {
	if !safePattern.MatchString(s) {
		return fmt.Errorf("%q is not a valid name: use letters, digits, '.', '_' and '-'", s)
	}
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("%q is not a valid name: it must not start with '-'", s)
	}
	switch s {
	case "null", "Null", "NULL", "~":
		// A plain key YAML reads as null: the service would vanish.
		return fmt.Errorf("%q is not a valid name: YAML reads it as null", s)
	}
	return nil
}

// ValidateHost checks a remote host; empty is allowed (default host).
func ValidateHost(s string) error {
	if s == "" {
		return nil
	}
	if !safePattern.MatchString(s) {
		return fmt.Errorf("%q is not a valid host: use letters, digits, '.', '_' and '-'", s)
	}
	if strings.HasPrefix(s, "-") {
		// Also keeps the host from reading as an ssh option.
		return fmt.Errorf("%q is not a valid host: it must not start with '-'", s)
	}
	return nil
}

// ParsePort parses a TCP port number typed by the user.
func ParsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("%d is out of range: must be 1-65535", n)
	}
	return n, nil
}
