package tunnel

import (
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"log/slog"
	"net"
	"net/url"
)

type TcpForwardConfig struct {
	ServiceUrl string
	Instance   string
	Port       int
	Project    string
	Zone       string
	Upstream   string
}

func StartClient(ctx context.Context, addr string, c TcpForwardConfig) error {
	// cloud run
	if c.ServiceUrl != "" {
		u, err := url.Parse(c.ServiceUrl)
		if err != nil {
			return err
		}

		ts, err := findTokenSource(ctx, c.ServiceUrl)
		if err != nil {
			return err
		}

		p := tcpForward{
			addr:     addr,
			upstream: c.Upstream,
			dialer:   NewCloudRunRemoteDialer(ts, u),
		}

		return p.start()
	}

	// iap
	{
		ts, err := google.DefaultTokenSource(ctx)
		if err != nil {
			return err
		}

		p := tcpForward{
			addr:     addr,
			upstream: c.Upstream,
			dialer:   NewIAPRemoteDialer(ts, c.Instance, c.Port, c.Project, c.Zone),
		}

		return p.start()
	}
}

type tcpForward struct {
	addr     string
	upstream string
	dialer   Dialer
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
	dst, err := tp.dialer.Dial("tcp", tp.upstream)
	if err != nil {
		slog.Error("Unable to dial upstream", "addr", tp.upstream, "err", err)
		return
	}
	slog.Info("Dialed remote upstream", "addr", tp.upstream)
	pipe(conn, dst)
}
