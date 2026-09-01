package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/spf13/cobra"
)

const defaultConfigTemplate = `services:
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

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open configuration file in a text editor",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}

		if err := ensureConfigFile(path); err != nil {
			return err
		}

		editor, editorArgs := resolveEditor()
		editorArgs = append(editorArgs, path)

		// Editor is resolved from $VISUAL/$EDITOR (user's own env) or a hard-coded
		// allowlist — the same pattern used by git, kubectl, crontab, etc.
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		c := exec.Command(editor, editorArgs...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("editor %q failed: %w", editor, err)
		}
		return nil
	},
}

func ensureConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0o600); err != nil {
		return fmt.Errorf("cannot create config file: %w", err)
	}
	return nil
}

func resolveEditor() (string, []string) {
	if v := os.Getenv("VISUAL"); v != "" {
		return v, nil
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v, nil
	}
	if runtime.GOOS == "windows" {
		return "notepad.exe", nil
	}
	for _, candidate := range []string{"nano", "vim", "vi"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	return "vi", nil
}

func init() {
	rootCmd.AddCommand(editCmd)
}
