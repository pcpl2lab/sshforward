package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Injected at build time via -ldflags "-X github.com/pcpl2/sshforward/cmd.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "sshforward %s\ncommit: %s\nbuilt:  %s\nos/arch: %s/%s\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH)
		return nil
	},
}

func init() {
	rootCmd.Version = version // enables `sshforward --version`
	rootCmd.AddCommand(versionCmd)
}
