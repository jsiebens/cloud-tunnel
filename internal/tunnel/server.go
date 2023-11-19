package tunnel

import (
	"fmt"
	"log/slog"
)

func StartServer(addr string) error {
	s := &Server{}
	return s.start(addr)
}

type Server struct {
}

func (s *Server) start(addr string) error {
	slog.Info(fmt.Sprintf("Listening on %s", addr))
	tunnel := &tunnelServer{}
	return tunnel.listenAndServe(addr)
}
