package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/pcpl2/sshforward/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config not found. Create ~/.sshforward/config.yaml\n\nExample:\nservices:\n  mysql:\n    remote_port: 3306\n  gitea:\n    remote_host: 10.0.0.5\n    ports:\n      - name: web\n        remote_port: 3000\n      - name: ssh\n        remote_port: 2222")
		}

		names := make([]string, 0, len(cfg.Services))
		for n := range cfg.Services {
			names = append(names, n)
		}
		sort.Strings(names)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "SERVICE\tPORT\tREMOTE HOST\tREMOTE PORT\tLOCAL PORT")

		for _, name := range names {
			svc := cfg.Services[name]
			for _, p := range svc.Ports {
				localPort := "auto"
				if p.LocalPort > 0 {
					localPort = fmt.Sprintf("%d", p.LocalPort)
				}
				portName := "-"
				if p.Name != "" {
					portName = p.Name
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", name, portName, p.RemoteHost, p.RemotePort, localPort)
			}
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
