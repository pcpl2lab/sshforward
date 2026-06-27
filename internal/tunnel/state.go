package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TunnelState struct {
	Host       string    `json:"host"`
	Service    string    `json:"service"`
	PortName   string    `json:"port_name,omitempty"` // empty for single-port services
	PID        int       `json:"pid"`
	LocalPort  int       `json:"local_port"`
	RemoteHost string    `json:"remote_host"`
	RemotePort int       `json:"remote_port"`
	StartedAt  time.Time `json:"started_at"`
}

func TunnelsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".sshforward", "tunnels"), nil
}

// StateKey returns a unique key for a tunnel: "host-service" or "host-service-portname"
func StateKey(host, service, portName string) string {
	if portName == "" {
		return fmt.Sprintf("%s-%s", host, service)
	}
	return fmt.Sprintf("%s-%s-%s", host, service, portName)
}

func StatePath(dir, host, service string) string {
	return filepath.Join(dir, StateKey(host, service, "")+".json")
}

func StatePathWithPort(dir, host, service, portName string) string {
	return filepath.Join(dir, StateKey(host, service, portName)+".json")
}

func SaveState(path string, state *TunnelState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("cannot create tunnels directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("cannot write state file: %w", err)
	}
	return nil
}

func LoadState(path string) (*TunnelState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read state file: %w", err)
	}
	var state TunnelState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("cannot parse state file: %w", err)
	}
	return &state, nil
}

func ListStates(dir string) ([]*TunnelState, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read tunnels directory: %w", err)
	}

	var states []*TunnelState
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		state, err := LoadState(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		states = append(states, state)
	}
	return states, nil
}

// ListServiceStates returns all states for a given host/service combo (all port names).
func ListServiceStates(dir, host, service string) ([]*TunnelState, error) {
	allStates, err := ListStates(dir)
	if err != nil {
		return nil, err
	}
	var matched []*TunnelState
	for _, s := range allStates {
		if s.Host == host && s.Service == service {
			matched = append(matched, s)
		}
	}
	return matched, nil
}

func RemoveState(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove state file: %w", err)
	}
	return nil
}
