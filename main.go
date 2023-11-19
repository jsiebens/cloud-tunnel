package main

import (
	"github.com/jsiebens/cloud-tunnel/internal/tunnel"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
)

func main() {
	cmd := &cobra.Command{}

	cmd.AddCommand(serverCommand())
	cmd.AddCommand(tcpForwardCommand())
	cmd.AddCommand(httpProxyCommand())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func serverCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "server",
		SilenceUsage: true,
	}

	var addr string

	cmd.Flags().StringVarP(&addr, "listen-addr", "", ":7654", "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return tunnel.StartServer(addr)
	}

	return cmd
}

func tcpForwardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "tcp-forward",
		SilenceUsage: true,
	}

	var addr string
	var c = tunnel.TcpForwardConfig{}

	cmd.Flags().StringVarP(&addr, "listen-addr", "", "127.0.0.1:8080", "")
	cmd.Flags().StringVarP(&c.Upstream, "upstream", "", "", "")
	cmd.Flags().StringVarP(&c.ServiceUrl, "service-url", "", "", "")
	cmd.Flags().StringVarP(&c.Instance, "instance", "", "", "")
	cmd.Flags().IntVarP(&c.Port, "port", "", tunnel.DefaultServerPort, "")
	cmd.Flags().StringVarP(&c.Project, "project", "", "", "")
	cmd.Flags().StringVarP(&c.Zone, "zone", "", "", "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return tunnel.StartClient(cmd.Context(), addr, c)
	}

	return cmd
}

func httpProxyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "http-proxy",
		SilenceUsage: true,
	}

	var (
		addr       string
		configFile string
		rule       = tunnel.Rule{Tunnel: tunnel.TunnelConfig{}}
	)

	cmd.Flags().StringVarP(&addr, "listen-addr", "", "127.0.0.1:8080", "")
	cmd.Flags().StringVarP(&rule.Tunnel.ServiceUrl, "service-url", "", "", "")
	cmd.Flags().StringVarP(&rule.Tunnel.Instance, "instance", "", "", "")
	cmd.Flags().IntVarP(&rule.Tunnel.Port, "port", "", tunnel.DefaultServerPort, "")
	cmd.Flags().StringVarP(&rule.Tunnel.Project, "project", "", "", "")
	cmd.Flags().StringVarP(&rule.Tunnel.Zone, "zone", "", "", "")
	cmd.Flags().StringSliceVarP(&rule.Upstreams, "upstream", "", []string{}, "")
	cmd.Flags().StringVarP(&configFile, "config", "", "", "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		config := tunnel.HttpProxyConfig{}

		if configFile == "" {
			config.Rules = []tunnel.Rule{rule}
		} else {
			content, err := os.ReadFile(configFile)
			if err != nil {
				return err
			}
			if err = yaml.Unmarshal(content, &config); err != nil {
				return err
			}
		}

		return tunnel.StartHttpProxy(cmd.Context(), addr, config)
	}

	return cmd
}
