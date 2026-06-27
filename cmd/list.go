package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/pcpl2/sshforward/internal/tunnel"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active SSH tunnels",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tunnelsDir, err := tunnel.TunnelsDir()
		if err != nil {
			return err
		}

		states, err := tunnel.ListStates(tunnelsDir)
		if err != nil {
			return err
		}

		if len(states) == 0 {
			fmt.Println("No active tunnels")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "HOST\tSERVICE\tPORT\tLOCAL PORT\tREMOTE\tPID\tSTATUS")

		var deadTunnels []*tunnel.TunnelState
		for _, s := range states {
			remote := fmt.Sprintf("%s:%d", s.RemoteHost, s.RemotePort)
			portName := "-"
			if s.PortName != "" {
				portName = s.PortName
			}

			if !tunnel.IsProcessAlive(s.PID) {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%d\t%s\n",
					s.Host, s.Service, portName, s.LocalPort, remote, s.PID, "DEAD")
				deadTunnels = append(deadTunnels, s)
				continue
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%d\t%s\n",
				s.Host, s.Service, portName, s.LocalPort, remote, s.PID, "active")
		}
		w.Flush()

		// Show logs for dead tunnels and clean them up
		for _, s := range deadTunnels {
			log := tunnel.ReadLog(tunnelsDir, s.Host, s.Service, s.PortName)
			if log != "" && log != "(empty log)" {
				label := fmt.Sprintf("%s/%s", s.Host, s.Service)
				if s.PortName != "" {
					label = fmt.Sprintf("%s/%s/%s", s.Host, s.Service, s.PortName)
				}
				fmt.Fprintf(os.Stderr, "\n--- SSH log for %s (DEAD) ---\n%s\n", label, log)
			}
			statePath := tunnel.StatePathWithPort(tunnelsDir, s.Host, s.Service, s.PortName)
			tunnel.RemoveState(statePath)
			os.Remove(tunnel.LogPath(tunnelsDir, s.Host, s.Service, s.PortName))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
