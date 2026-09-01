package cmd

import (
	"fmt"
	"os"

	"github.com/pcpl2lab/sshforward/internal/tunnel"
	"github.com/spf13/cobra"
)

var stopAll bool

var stopCmd = &cobra.Command{
	Use:   "stop [<host> <service>]",
	Short: "Stop an SSH port forwarding tunnel",
	Args: func(cmd *cobra.Command, args []string) error {
		if stopAll {
			if len(args) != 0 {
				return fmt.Errorf("--all flag does not accept arguments")
			}
			return nil
		}
		if len(args) != 2 {
			return fmt.Errorf("requires exactly 2 arguments: <host> <service>")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		tunnelsDir, err := tunnel.TunnelsDir()
		if err != nil {
			return err
		}

		if stopAll {
			stopped, errs := tunnel.StopAll(tunnelsDir)
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "Error: %v\n", e)
			}
			fmt.Fprintf(os.Stdout, "Stopped %d tunnel(s)", stopped)
			if len(errs) > 0 {
				fmt.Fprintf(os.Stdout, ", %d failed", len(errs))
				fmt.Fprintln(os.Stdout)
				return fmt.Errorf("failed to stop %d tunnel(s)", len(errs))
			}
			fmt.Fprintln(os.Stdout)
			return nil
		}

		host := args[0]
		service := args[1]
		if err := tunnel.Stop(tunnelsDir, host, service); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Tunnel %s/%s stopped\n", host, service)
		return nil
	},
}

func init() {
	stopCmd.Flags().BoolVar(&stopAll, "all", false, "Stop all active tunnels")
	rootCmd.AddCommand(stopCmd)
}
