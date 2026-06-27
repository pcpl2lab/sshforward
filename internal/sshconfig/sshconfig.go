package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sshcfg "github.com/kevinburke/ssh_config"
)

func DefaultSSHConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

func ListHostsFromPath(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read SSH config: %w", err)
	}
	defer f.Close()

	cfg, err := sshcfg.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("invalid SSH config: %w", err)
	}

	var hosts []string
	for _, host := range cfg.Hosts {
		for _, pattern := range host.Patterns {
			name := pattern.String()
			if strings.ContainsAny(name, "*?") {
				continue
			}
			if name == "" {
				continue
			}
			hosts = append(hosts, name)
		}
	}
	return hosts, nil
}

func ListHosts() ([]string, error) {
	path, err := DefaultSSHConfigPath()
	if err != nil {
		return nil, err
	}
	return ListHostsFromPath(path)
}

func ValidateHostFromPath(path, host string) error {
	hosts, err := ListHostsFromPath(path)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if h == host {
			return nil
		}
	}
	return fmt.Errorf("host %q not found in %s. Available hosts: %s", host, path, strings.Join(hosts, ", "))
}

func ValidateHost(host string) error {
	path, err := DefaultSSHConfigPath()
	if err != nil {
		return err
	}
	return ValidateHostFromPath(path, host)
}
