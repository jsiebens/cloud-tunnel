package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

func StartServer(addr string, timeout time.Duration, allowedUpstreams []string) error {
	slog.Info(fmt.Sprintf("Listening on %s", addr))
	tunnel := newTunnelServer(timeout, allowedUpstreams)
	return tunnel.listenAndServe(addr)
}

func newTunnelServer(timeout time.Duration, allowedUpstreams []string) *tunnelServer {
	dialer := NewDefaultDialer(timeout)

	if len(allowedUpstreams) == 0 {
		return &tunnelServer{allowedUpstreams: []proxyUpstream{newProxyUpstream("*", dialer)}}
	}

	var upstreams []proxyUpstream
	for _, u := range allowedUpstreams {
		upstreams = append(upstreams, newProxyUpstream(u, dialer))
	}

	return &tunnelServer{allowedUpstreams: upstreams}
}

type tunnelServer struct {
	allowedUpstreams []proxyUpstream
}

func (s *tunnelServer) listenAndServe(addr string) error {
	server := &http.Server{Addr: addr, Handler: s}
	return server.ListenAndServe()
}

func (s *tunnelServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	target := req.Header.Get(UpstreamHeaderName)
	if len(target) == 0 {
		http.Error(w, "missing target header", http.StatusBadRequest)
		return
	}

	dialer := s.getDialer(target)

	if dialer == nil {
		http.Error(w, "upstream not allowed", http.StatusForbidden)
		return
	}

	conn, err := s.hijackConnection(w, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.handleConnection(conn, target, dialer)
}

func (s *tunnelServer) hijackConnection(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	next := r.Header.Get("Upgrade")
	if next == "" {
		return nil, errors.New("missing next protocol")
	}
	if next != "websocket" {
		return nil, errors.New("unknown next protocol")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("make request over HTTP/1")
	}

	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "upgrade")
	w.WriteHeader(http.StatusSwitchingProtocols)

	conn, brw, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijacking client connection: %w", err)
	}

	if err := brw.Flush(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("flushing hijacked HTTP buffer: %w", err)
	}

	return conn, nil
}

func (s *tunnelServer) handleConnection(conn net.Conn, target string, dialer Dialer) {
	defer conn.Close()
	dst, err := dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		slog.Error("Unable to dial upstream", "addr", target, "err", err)
		return
	}
	slog.Info("Dialed upstream", "addr", target)
	defer dst.Close()
	pipe(conn, dst)
}

func (s *tunnelServer) getDialer(target string) Dialer {
	for _, u := range s.allowedUpstreams {
		if u.matches(target) {
			return u.dialer
		}
	}

	return nil
}

func pipe(from, to io.ReadWriteCloser) {
	cp := func(dst io.Writer, src io.Reader, cancel context.CancelFunc) {
		_, _ = io.Copy(dst, src)
		cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())

	go cp(from, to, cancel)
	go cp(to, from, cancel)

	<-ctx.Done()
	_ = from.Close()
	_ = to.Close()
}
