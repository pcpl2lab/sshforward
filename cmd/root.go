package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

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
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
