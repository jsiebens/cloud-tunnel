package main

import (
	"fmt"
	"github.com/jsiebens/cloud-tunnel/internal/version"
	"github.com/jsiebens/cloud-tunnel/pkg/proxy"
	"github.com/jsiebens/cloud-tunnel/pkg/remotedialer"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

func main() {
	cmd := &cobra.Command{}

	cmd.AddCommand(versionCommand())
	cmd.AddCommand(serverCommand())
	cmd.AddCommand(tcpForwardCommand())
	cmd.AddCommand(proxyCommand())

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
	var timeout time.Duration
	var allowedUpstreams []string

	cmd.Flags().StringVarP(&addr, "listen-addr", "", ":7654", "")
	cmd.Flags().DurationVarP(&timeout, "dial-timeout", "", proxy.DefaultTimeout, "")
	cmd.Flags().StringSliceVarP(&allowedUpstreams, "allowed-upstream", "", []string{}, "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return proxy.StartServer(addr, timeout, allowedUpstreams)
	}

	return cmd
}

func tcpForwardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "tcp-forward",
		SilenceUsage: true,
	}

	var addr string
	var c = proxy.TcpForwardConfig{}

	cmd.Flags().StringVarP(&addr, "listen-addr", "", "127.0.0.1:8080", "")
	cmd.Flags().StringVarP(&c.Upstream, "upstream", "", "", "")
	cmd.Flags().StringVarP(&c.ServiceUrl, "service-url", "", "", "")
	cmd.Flags().StringVarP(&c.ServiceAccount, "service-account", "", "", "")
	cmd.Flags().StringVarP(&c.Instance, "instance", "", "", "")
	cmd.Flags().IntVarP(&c.Port, "port", "", remotedialer.DefaultServerPort, "")
	cmd.Flags().StringVarP(&c.Project, "project", "", "", "")
	cmd.Flags().StringVarP(&c.Zone, "zone", "", "", "")
	cmd.Flags().BoolVarP(&c.MuxEnabled, "mux", "", false, "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return proxy.StartTcpForward(cmd.Context(), addr, c)
	}

	return cmd
}

func proxyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "proxy",
		SilenceUsage: true,
	}

	var (
		addr       string
		configFile string
		rule       = proxy.Rule{Tunnel: proxy.Tunnel{}}
	)

	cmd.Flags().StringVarP(&addr, "listen-addr", "", "127.0.0.1:8080", "")
	cmd.Flags().StringVarP(&rule.Tunnel.ServiceUrl, "service-url", "", "", "")
	cmd.Flags().StringVarP(&rule.Tunnel.ServiceAccount, "service-account", "", "", "")
	cmd.Flags().StringVarP(&rule.Tunnel.Instance, "instance", "", "", "")
	cmd.Flags().IntVarP(&rule.Tunnel.Port, "port", "", remotedialer.DefaultServerPort, "")
	cmd.Flags().StringVarP(&rule.Tunnel.Project, "project", "", "", "")
	cmd.Flags().StringVarP(&rule.Tunnel.Zone, "zone", "", "", "")
	cmd.Flags().BoolVarP(&rule.Tunnel.MuxEnabled, "mux", "", false, "")
	cmd.Flags().StringSliceVarP(&rule.Upstreams, "upstream", "", []string{}, "")
	cmd.Flags().StringVarP(&configFile, "config", "", "", "")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		config := proxy.ProxyConfig{}

		if configFile == "" {
			config.Rules = []proxy.Rule{rule}
		} else {
			content, err := os.ReadFile(configFile)
			if err != nil {
				return err
			}
			if err = yaml.Unmarshal(content, &config); err != nil {
				return err
			}
		}

		return proxy.StartProxy(cmd.Context(), addr, config)
	}

	return cmd
}

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "version",
		Short:        "Display version information",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			clientVersion, clientRevision := version.GetReleaseInfo()
			fmt.Printf("Version:   %s\nRevision:  %s\n", clientVersion, clientRevision)
		},
	}

	return cmd
}
