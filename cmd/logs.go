package cmd

import (
	"fmt"
	"os"

	"github.com/pcpl2lab/sshforward/internal/tunnel"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [<host> <service>]",
	Short: "Show SSH stderr logs for tunnels",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(_ *cobra.Command, args []string) error {
		tunnelsDir, err := tunnel.TunnelsDir()
		if err != nil {
			return err
		}

		var states []*tunnel.TunnelState
		if len(args) == 2 {
			states, err = tunnel.ListServiceStates(tunnelsDir, args[0], args[1])
		} else {
			states, err = tunnel.ListStates(tunnelsDir)
		}
		if err != nil {
			return err
		}

		if len(states) == 0 {
			fmt.Println("No tunnels found")
			return nil
		}

		for _, s := range states {
			label := fmt.Sprintf("%s/%s", s.Host, s.Service)
			if s.PortName != "" {
				label = fmt.Sprintf("%s/%s/%s", s.Host, s.Service, s.PortName)
			}
			status := "active"
			if !tunnel.IsTunnelAlive(s.PID) {
				status = "DEAD"
			}
			log := tunnel.ReadLog(tunnelsDir, s.Host, s.Service, s.PortName)
			fmt.Fprintf(os.Stdout, "--- %s (PID: %d, %s) ---\n", label, s.PID, status)
			if log == "" {
				fmt.Println("(no log)")
			} else {
				fmt.Println(log)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
