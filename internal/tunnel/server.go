package tunnel

import (
	"fmt"
	"github.com/hashicorp/yamux"
	"log/slog"
	"net"
)

func StartServer(addr string) error {
	s := &Server{}
	return s.start(addr)
}

type Server struct {
}

func (s *Server) start(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("Listening on %s", addr))

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	server, err := yamux.Server(conn, nil)
	if err != nil {
		return
	}
	defer server.Close()

	tunnel := &tunnelServer{}
	_ = tunnel.serve(server)
}
