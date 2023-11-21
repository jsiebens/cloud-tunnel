package tunnel

import (
	"fmt"
	"log/slog"
	"time"
)

func StartServer(addr string, timeout time.Duration) error {
	slog.Info(fmt.Sprintf("Listening on %s", addr))
	tunnel := &tunnelServer{timeout: timeout}
	return tunnel.listenAndServe(addr)
}
