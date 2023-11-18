package tunnel

import (
	"fmt"
	"github.com/hashicorp/yamux"
	"log/slog"
	"net"
	"os"
)

func StartServer(addr string) error {
	s := &Server{}
	return s.start(addr)
}

type Server struct {
}

func (s *Server) start(addr string) error {
	// if running as a Cloud Run service, don't use mux
	if os.Getenv("K_SERVICE") != "" {
		slog.Info(fmt.Sprintf("Listening on %s", addr))
		tunnel := &tunnelServer{}
		return tunnel.listenAndServe(addr)
	}

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
