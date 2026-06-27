package cmd

import (
	"fmt"
	"os"

	"github.com/pcpl2/sshforward/internal/config"
	"github.com/pcpl2/sshforward/internal/sshconfig"
	"github.com/pcpl2/sshforward/internal/tunnel"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <host> <service>",
	Short: "Start an SSH port forwarding tunnel",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := args[0]
		service := args[1]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config not found. Create ~/.sshforward/config.yaml with your service definitions")
		}

		svc, err := cfg.GetService(service)
		if err != nil {
			names := make([]string, 0, len(cfg.Services))
			for n := range cfg.Services {
				names = append(names, n)
			}
			return fmt.Errorf("unknown service %q. Available services: %v", service, names)
		}

		if err := sshconfig.ValidateHost(host); err != nil {
			return err
		}

		tunnelsDir, err := tunnel.TunnelsDir()
		if err != nil {
			return err
		}

		// Convert config ports to tunnel ports
		ports := make([]tunnel.PortForward, len(svc.Ports))
		for i, p := range svc.Ports {
			ports[i] = tunnel.PortForward{
				Name:       p.Name,
				RemoteHost: p.RemoteHost,
				RemotePort: p.RemotePort,
				LocalPort:  p.LocalPort,
			}
		}

		states, err := tunnel.Start(&tunnel.StartOptions{
			Host:       host,
			Service:    service,
			Ports:      ports,
			TunnelsDir: tunnelsDir,
		})
		if err != nil {
			return err
		}

		for _, s := range states {
			label := fmt.Sprintf("%s/%s", host, service)
			if s.PortName != "" {
				label = fmt.Sprintf("%s/%s/%s", host, service, s.PortName)
			}
			fmt.Fprintf(os.Stdout, "Tunnel %s started: localhost:%d -> %s:%d (PID: %d)\n",
				label, s.LocalPort, s.RemoteHost, s.RemotePort, s.PID)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
