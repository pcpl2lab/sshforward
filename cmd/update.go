package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/pcpl2lab/sshforward/internal/update"
	"github.com/spf13/cobra"
)

// noUpdateCheckEnv disables the once-a-day background check when set to any
// non-empty value.
const noUpdateCheckEnv = "SSHFORWARD_NO_UPDATE_CHECK"

// updateTimeout bounds an explicit `sshforward update`, which may download an
// archive and so needs far longer than a version lookup.
const updateTimeout = 5 * time.Minute

// quietCommands never trigger the background check: version and update report
// on updates themselves, while help and completion must stay instant, offline
// and byte-for-byte predictable.
var quietCommands = map[string]bool{
	"version":    true,
	"update":     true,
	"help":       true,
	"completion": true,
}

// backgroundCheckAllowed reports whether the once-a-day check may run for this
// invocation. Any opt-out wins over every other consideration.
func backgroundCheckAllowed(command, currentVersion, envDisable string, configEnabled bool) bool {
	if currentVersion == devVersion || envDisable != "" || !configEnabled {
		return false
	}
	return !quietCommands[command]
}

var updateCheckOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for a newer release and install it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()

		if version == devVersion {
			fmt.Fprintf(out, "This is a development build, so there is nothing to compare against.\nReleases: %s\n", update.ReleasesURL)
			return nil
		}

		dir, err := sshforwardDir()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), updateTimeout)
		defer cancel()

		res, err := update.Check(ctx, update.CheckOptions{
			CurrentVersion: version,
			Dir:            dir,
			Client:         newUpdateClient(),
			Force:          true, // an explicit check must never answer from cache
		})
		if err != nil {
			return fmt.Errorf("cannot check for updates: %w", err)
		}

		if !res.Available {
			fmt.Fprintf(out, "sshforward %s is up to date.\n", version)
			return nil
		}
		fmt.Fprintf(out, "A newer release is available: %s (you have %s)\n", res.Latest, version)

		if updateCheckOnly {
			fmt.Fprintf(out, "Release notes: %s\n", res.Release.URL)
			return nil
		}

		// Replacing a file a package manager owns breaks its manifest and is
		// undone by the next upgrade, so hand the user its own command instead.
		if src := update.DetectSource(); src.Managed() {
			fmt.Fprintf(out, "\nThis copy was installed with %s.\nTo update: %s\n", src, src.UpgradeCommand())
			return nil
		}

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot locate the running binary: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if err := checkWritable(exe); err != nil {
			return err
		}

		fmt.Fprintf(out, "Downloading %s...\n", update.AssetName(runtime.GOOS, runtime.GOARCH))
		if err := update.Apply(ctx, update.ApplyOptions{Release: res.Release, TargetPath: exe}); err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated to %s.\n", res.Latest)
		return nil
	},
}

// checkWritable rejects an update the process could not finish. Path detection
// is a heuristic; file permissions are not, so this is the backstop that keeps
// a misdetected system install from being half-replaced.
func checkWritable(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w\n\nReinstall from %s, or run the update with the rights to replace that file", path, err, update.ReleasesURL)
	}
	return f.Close()
}

// sshforwardDir returns ~/.sshforward, where the update cache lives alongside
// the config and the tunnel state.
func sshforwardDir() (string, error) {
	path, err := config.DefaultConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

// newUpdateClient builds the GitHub client. GITHUB_TOKEN is honoured because it
// raises the anonymous rate limit, which a shared office IP can otherwise hit.
func newUpdateClient() *update.Client {
	return &update.Client{
		HTTP:      &http.Client{Timeout: update.DefaultTimeout},
		UserAgent: "sshforward/" + version,
		Token:     os.Getenv("GITHUB_TOKEN"),
	}
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Only report whether a newer release exists")
	rootCmd.AddCommand(updateCmd)
}
