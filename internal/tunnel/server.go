package tunnel

import (
	"fmt"
	"github.com/hashicorp/yamux"
	"log/slog"
	"net"
	"os"
	"time"
)

func StartServer(addr string, timeout time.Duration) error {
	s := &Server{timeout}
	return s.start(addr)
}

type Server struct {
	timeout time.Duration
}

func (s *Server) start(addr string) error {
	// if running as a Cloud Run service, don't use mux
	if os.Getenv("K_SERVICE") != "" {
		slog.Info(fmt.Sprintf("Listening on %s in standard mode", addr))
		tunnel := &tunnelServer{timeout: s.timeout}
		return tunnel.listenAndServe(addr)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("Listening on %s in mux mode", addr))

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

	tunnel := &tunnelServer{timeout: s.timeout}
	_ = tunnel.serve(server)
}
