package proxy

import (
	"context"
	"fmt"
	"github.com/jsiebens/cloud-tunnel/pkg/remotedialer"
	"log/slog"
	"net"
	"net/url"
)

type TcpForwardConfig struct {
	ServiceUrl     string
	ServiceAccount string
	Instance       string
	Port           int
	Project        string
	Zone           string
	Upstream       string
	MuxEnabled     bool
}

func StartTcpForward(ctx context.Context, addr string, c TcpForwardConfig) error {
	// cloud run
	if c.ServiceUrl != "" {
		u, err := url.Parse(c.ServiceUrl)
		if err != nil {
			return err
		}

		ts, err := idTokenSource(ctx, c.ServiceUrl, c.ServiceAccount)
		if err != nil {
			return err
		}

		p := tcpForward{
			addr:     addr,
			upstream: c.Upstream,
			dialer:   remotedialer.RemoteDialer(ts, u, c.MuxEnabled),
		}

		return p.start()
	}

	// iap
	{
		ts, err := tokenSource(ctx, c.ServiceAccount)
		if err != nil {
			return err
		}

		p := tcpForward{
			addr:     addr,
			upstream: c.Upstream,
			dialer:   remotedialer.IAPRemoteDialer(ts, c.Instance, c.Port, c.Project, c.Zone, c.MuxEnabled),
		}

		return p.start()
	}
}

type tcpForward struct {
	addr     string
	upstream string
	dialer   remotedialer.Dialer
}

func (tp *tcpForward) start() error {
	listen, err := net.Listen("tcp", tp.addr)
	if err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("Listening on %s", tp.addr))

	for {
		conn, err := listen.Accept()
		if err != nil {
			return err
		}

		go tp.handleConnection(conn)
	}
}

func (tp *tcpForward) handleConnection(conn net.Conn) {
	defer conn.Close()
	dst, err := tp.dialer.DialContext(context.Background(), "tcp", tp.upstream)
	if err != nil {
		slog.Error("Unable to dial upstream", "addr", tp.upstream, "err", err)
		return
	}
	slog.Info("Dialed remote upstream", "addr", tp.upstream)
	pipe(conn, dst)
}
