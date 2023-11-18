package main

import (
	"github.com/jsiebens/cloud-tunnel/internal/tunnel"
	"github.com/spf13/cobra"
	"os"
)

func main() {
	cmd := &cobra.Command{}

	cmd.AddCommand(serverCommand())
	cmd.AddCommand(clientCommand())
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

func clientCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "tcp-forward",
		SilenceUsage: true,
	}

	var addr string
	var c = tunnel.TcpForwardConfig{}

	cmd.Flags().StringVarP(&addr, "listen-addr", "", "127.0.0.1:8080", "")
	cmd.Flags().StringVarP(&c.Upstream, "upstream", "", "", "")
	cmd.Flags().StringVarP(&c.Instance, "instance", "", "", "")
	cmd.Flags().IntVarP(&c.Port, "port", "", 7654, "")
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

	var addr string
	var c = tunnel.HttpProxyConfig{}

	cmd.Flags().StringVarP(&addr, "listen-addr", "", "127.0.0.1:8080", "")
	cmd.Flags().StringVarP(&c.Instance, "instance", "", "", "")
	cmd.Flags().IntVarP(&c.Port, "port", "", 7654, "")
	cmd.Flags().StringVarP(&c.Project, "project", "", "", "")
	cmd.Flags().StringVarP(&c.Zone, "zone", "", "", "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return tunnel.StartHttpProxy(cmd.Context(), addr, c)
	}

	return cmd
}
