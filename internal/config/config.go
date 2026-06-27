package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// PortMapping represents a single port forwarding within a service.
type PortMapping struct {
	Name       string // empty for single-port services
	RemoteHost string
	RemotePort int
	LocalPort  int // 0 = auto (random free port)
}

// Service represents a named service with one or more port forwardings.
type Service struct {
	Ports []PortMapping
}

type Config struct {
	Services map[string]Service `yaml:"services"`
}

// --- raw types for YAML parsing with unknown field detection ---

type rawPort struct {
	Name       string      `yaml:"name"`
	RemoteHost string      `yaml:"remote_host"`
	RemotePort int         `yaml:"remote_port"`
	LocalPort  interface{} `yaml:"local_port"`
}

type rawService struct {
	RemoteHost string      `yaml:"remote_host"`
	RemotePort int         `yaml:"remote_port"`
	LocalPort  interface{} `yaml:"local_port"`
	Ports      []rawPort   `yaml:"ports"`
}

type rawConfig struct {
	Services map[string]rawService `yaml:"services"`
}

// known top-level keys and service-level keys for unknown field detection
var knownServiceKeys = map[string]bool{
	"remote_host": true,
	"remote_port": true,
	"local_port":  true,
	"ports":       true,
}

var knownPortKeys = map[string]bool{
	"name":        true,
	"remote_host": true,
	"remote_port": true,
	"local_port":  true,
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".sshforward", "config.yaml"), nil
}

func Load() (*Config, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFromPath(path)
}

func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	// First pass: check for unknown fields using raw yaml.Node
	if err := checkUnknownFields(data); err != nil {
		return nil, err
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	cfg := &Config{
		Services: make(map[string]Service, len(raw.Services)),
	}

	for name, svc := range raw.Services {
		if len(svc.Ports) > 0 && svc.RemotePort != 0 {
			return nil, fmt.Errorf("service %q: cannot use both 'remote_port' and 'ports'", name)
		}

		if len(svc.Ports) > 0 {
			// Multi-port service
			ports, err := parseMultiPorts(name, svc.RemoteHost, svc.Ports)
			if err != nil {
				return nil, err
			}
			cfg.Services[name] = Service{Ports: ports}
		} else {
			// Single-port service (backward compatible)
			if svc.RemotePort == 0 {
				return nil, fmt.Errorf("service %q: remote_port is required (or use 'ports' for multi-port)", name)
			}
			localPort, err := parseLocalPort(svc.LocalPort, name, "")
			if err != nil {
				return nil, err
			}
			// Default: local_port = remote_port (unless explicitly set to 'auto')
			if localPort == -1 {
				localPort = svc.RemotePort
			}
			remoteHost := svc.RemoteHost
			if remoteHost == "" {
				remoteHost = "127.0.0.1"
			}
			cfg.Services[name] = Service{
				Ports: []PortMapping{{
					Name:       "",
					RemoteHost: remoteHost,
					RemotePort: svc.RemotePort,
					LocalPort:  localPort,
				}},
			}
		}
	}

	return cfg, nil
}

func parseMultiPorts(serviceName, defaultRemoteHost string, rawPorts []rawPort) ([]PortMapping, error) {
	if len(rawPorts) < 2 {
		return nil, fmt.Errorf("service %q: 'ports' must have at least 2 entries (use 'remote_port' for single port)", serviceName)
	}

	names := make(map[string]bool)
	var ports []PortMapping

	for i, rp := range rawPorts {
		if rp.Name == "" {
			return nil, fmt.Errorf("service %q: port entry %d: 'name' is required for multi-port services", serviceName, i+1)
		}
		if names[rp.Name] {
			return nil, fmt.Errorf("service %q: duplicate port name %q", serviceName, rp.Name)
		}
		names[rp.Name] = true

		if rp.RemotePort == 0 {
			return nil, fmt.Errorf("service %q: port %q: remote_port is required", serviceName, rp.Name)
		}

		localPort, err := parseLocalPort(rp.LocalPort, serviceName, rp.Name)
		if err != nil {
			return nil, err
		}
		// Default: local_port = remote_port (unless explicitly set to 'auto')
		if localPort == -1 {
			localPort = rp.RemotePort
		}

		remoteHost := rp.RemoteHost
		if remoteHost == "" {
			remoteHost = defaultRemoteHost
		}
		if remoteHost == "" {
			remoteHost = "127.0.0.1"
		}

		ports = append(ports, PortMapping{
			Name:       rp.Name,
			RemoteHost: remoteHost,
			RemotePort: rp.RemotePort,
			LocalPort:  localPort,
		})
	}

	return ports, nil
}

// parseLocalPort parses the local_port value.
// Returns:
//
//	-1 = not specified (will default to remote_port)
//	 0 = explicit "auto" (random free port)
//	>0 = explicit port number
func parseLocalPort(v interface{}, serviceName, portName string) (int, error) {
	ctx := fmt.Sprintf("service %q", serviceName)
	if portName != "" {
		ctx = fmt.Sprintf("service %q port %q", serviceName, portName)
	}

	switch val := v.(type) {
	case int:
		if val == 0 {
			return 0, nil // explicit 0 = auto
		}
		return val, nil
	case float64:
		if int(val) == 0 {
			return 0, nil
		}
		return int(val), nil
	case string:
		if val == "auto" {
			return 0, nil
		}
		if val == "" {
			return -1, nil // empty string = not specified
		}
		return 0, fmt.Errorf("%s: local_port must be a number or 'auto', got %q", ctx, val)
	case nil:
		return -1, nil // not specified = will default to remote_port
	default:
		return 0, fmt.Errorf("%s: invalid local_port type", ctx)
	}
}

// checkUnknownFields parses the YAML as a generic structure and checks for unknown keys.
func checkUnknownFields(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil // let the main parser handle syntax errors
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}

	// Find the "services" mapping
	for i := 0; i < len(doc.Content)-1; i += 2 {
		if doc.Content[i].Value == "services" {
			servicesNode := doc.Content[i+1]
			if servicesNode.Kind != yaml.MappingNode {
				continue
			}
			// Iterate services
			for j := 0; j < len(servicesNode.Content)-1; j += 2 {
				serviceName := servicesNode.Content[j].Value
				serviceNode := servicesNode.Content[j+1]
				if serviceNode.Kind != yaml.MappingNode {
					continue
				}
				if err := checkServiceKeys(serviceName, serviceNode); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkServiceKeys(serviceName string, node *yaml.Node) error {
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		if !knownServiceKeys[key] {
			suggestions := suggestKey(key, knownServiceKeys)
			if suggestions != "" {
				return fmt.Errorf("service %q line %d: unknown field %q (did you mean %s?)",
					serviceName, node.Content[i].Line, key, suggestions)
			}
			return fmt.Errorf("service %q line %d: unknown field %q. Valid fields: remote_host, remote_port, local_port, ports",
				serviceName, node.Content[i].Line, key)
		}

		// Check port entries
		if key == "ports" {
			portsNode := node.Content[i+1]
			if portsNode.Kind == yaml.SequenceNode {
				for idx, portNode := range portsNode.Content {
					if portNode.Kind == yaml.MappingNode {
						if err := checkPortKeys(serviceName, idx+1, portNode); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func checkPortKeys(serviceName string, portIdx int, node *yaml.Node) error {
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		if !knownPortKeys[key] {
			suggestions := suggestKey(key, knownPortKeys)
			if suggestions != "" {
				return fmt.Errorf("service %q port %d line %d: unknown field %q (did you mean %s?)",
					serviceName, portIdx, node.Content[i].Line, key, suggestions)
			}
			return fmt.Errorf("service %q port %d line %d: unknown field %q. Valid fields: name, remote_host, remote_port, local_port",
				serviceName, portIdx, node.Content[i].Line, key)
		}
	}
	return nil
}

// suggestKey returns a suggestion if the key looks like a typo of a known key.
func suggestKey(key string, known map[string]bool) string {
	key = strings.ToLower(key)
	var suggestions []string
	for k := range known {
		// Simple similarity: same letters, just missing/extra underscores or similar
		if strings.ReplaceAll(key, "_", "") == strings.ReplaceAll(k, "_", "") {
			suggestions = append(suggestions, fmt.Sprintf("%q", k))
		}
	}
	sort.Strings(suggestions)
	return strings.Join(suggestions, " or ")
}

func (c *Config) GetService(name string) (Service, error) {
	svc, ok := c.Services[name]
	if !ok {
		return Service{}, fmt.Errorf("unknown service %q", name)
	}
	return svc, nil
}

// IsMultiPort returns true if the service has more than one port mapping.
func (s Service) IsMultiPort() bool {
	return len(s.Ports) > 1
}
