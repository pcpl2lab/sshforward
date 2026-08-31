package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/pcpl2/sshforward/internal/config"
)

const exampleConfig = `services:
  mysql:
    remote_port: 3306
  gitea:
    remote_host: 10.0.0.5
    ports:
      - name: web
        remote_port: 3000
      - name: ssh
        remote_port: 2222`

// loadConfig loads the user config. A missing file becomes actionable guidance;
// every other failure (YAML syntax, unknown fields, missing remote_port) is
// returned unchanged so the parser's diagnostics reach the user.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	path, pathErr := config.DefaultConfigPath()
	if pathErr != nil {
		path = "~/.sshforward/config.yaml"
	}
	return nil, fmt.Errorf("no config file at %s\n\nRun 'sshforward edit' to create one, for example:\n\n%s", path, exampleConfig)
}
