package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
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
