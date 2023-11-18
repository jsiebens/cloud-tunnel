package tunnel

import (
	"context"
	"fmt"
	"github.com/hashicorp/yamux"
	"log/slog"
	"net"
)

type TcpForwardConfig struct {
	Instance string
	Port     int
	Project  string
	Zone     string
	Upstream string
}

func StartClient(ctx context.Context, addr string, c TcpForwardConfig) error {
	conn, err := dial(ctx, c.Instance, c.Port, c.Project, c.Zone)
	if err != nil {
		return err
	}
	defer conn.Close()

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return err
	}
	defer session.Close()

	p := tcpForward{
		addr:     addr,
		upstream: c.Upstream,
		dial:     connectViaMux(session),
	}

	go p.start()

	<-session.CloseChan()
	return fmt.Errorf("session closed")
}

type tcpForward struct {
	addr     string
	upstream string
	dial     dialFunc
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
	dst, err := tp.dial("tcp", tp.upstream)
	if err != nil {
		slog.Error("Unable to dial upstream", "addr", tp.upstream, "err", err)
		return
	}
	slog.Info("Dialed remote upstream", "addr", tp.upstream)
	pipe(conn, dst)
}
