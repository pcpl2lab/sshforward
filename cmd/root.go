package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pcpl2/sshforward/internal/config"
	"github.com/pcpl2/sshforward/internal/update"
	"github.com/spf13/cobra"
)

// backgroundCheckGrace is how long a finished command waits for the update
// check it started. Long-running commands like start overlap it entirely;
// instant ones give up rather than make the user wait for a courtesy.
const backgroundCheckGrace = time.Second

// updateCheck carries the background result from PersistentPreRun to
// PersistentPostRun. It is nil when no check was started.
var updateCheck chan *update.Result

var rootCmd = &cobra.Command{
	Use:   "sshforward",
	Short: "SSH port forwarding manager",
	Long:  "Manage SSH local port forwarding tunnels with automatic port selection.",
	// Execute prints the error itself; without this cobra prints it a second time.
	SilenceErrors: true,
	// Runs after argument validation but before RunE, so usage is still shown
	// for genuine misuse while runtime failures stay a single-line error.
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true
		startUpdateCheck(cmd.Name())
	},
	// Only reached when the command succeeded, so a failure is never buried
	// under an update notice.
	PersistentPostRun: func(cmd *cobra.Command, _ []string) {
		reportUpdateCheck(cmd)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// startUpdateCheck kicks off the once-a-day release lookup in the background so
// it overlaps the command's own work instead of delaying it.
func startUpdateCheck(command string) {
	configEnabled := true
	if cfg, err := config.Load(); err == nil {
		configEnabled = cfg.UpdateCheck
	}
	if !backgroundCheckAllowed(command, version, os.Getenv(noUpdateCheckEnv), configEnabled) {
		return
	}
	dir, err := sshforwardDir()
	if err != nil {
		return
	}

	ch := make(chan *update.Result, 1)
	updateCheck = ch
	go func() {
		defer close(ch)
		ctx, cancel := context.WithTimeout(context.Background(), update.DefaultTimeout)
		defer cancel()

		// Every failure here is silent by design: being offline, behind a
		// proxy or rate-limited must not add noise to an unrelated command.
		if res, err := update.Check(ctx, update.CheckOptions{
			CurrentVersion: version,
			Dir:            dir,
			Client:         newUpdateClient(),
		}); err == nil {
			ch <- res
		}
	}()
}

// reportUpdateCheck prints the notice, if the check finished in time and found
// something. It writes to stderr so that list and config stay parsable.
func reportUpdateCheck(cmd *cobra.Command) {
	if updateCheck == nil {
		return
	}
	select {
	case res := <-updateCheck:
		if res != nil && res.Available {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"\nA newer sshforward is available: %s (you have %s). Run 'sshforward update'.\n",
				res.Latest, version)
		}
	case <-time.After(backgroundCheckGrace):
	}
}
