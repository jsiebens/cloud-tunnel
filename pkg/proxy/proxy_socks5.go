package proxy

import (
	"context"
	"github.com/jsiebens/cloud-tunnel/pkg/remotedialer"
	"log/slog"
	"net"
	"tailscale.com/net/socks5"
)

type socks5Proxy struct {
	dialer  remotedialer.Dialer
	targets proxyUpstreams
}

func (sp *socks5Proxy) serve(ln net.Listener) error {
	s := &socks5.Server{
		Dialer: sp.dialContext,
	}
	return s.Serve(ln)
}

func (sp *socks5Proxy) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	mode, dialer := sp.targets.getDialer(addr, sp.dialer)
	conn, err := dialer.DialContext(ctx, network, addr)

	if err != nil {
		slog.Error("Error dialing upstream", "addr", addr, "err", err)
		return conn, err
	}

	slog.Info("Dialed upstream", "addr", addr, "mode", mode)
	return conn, err
}
