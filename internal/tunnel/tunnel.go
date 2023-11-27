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

const (
	DefaultTimeout          = 5 * time.Second
	DefaultServerPort       = 7654
	authorizationHeaderName = "Authorization"
	upstreamHeaderName      = "X-Cloud-Tunnel-Upstream"
)

type tunnelServer struct {
	timeout time.Duration
}

func (s *tunnelServer) serve(l net.Listener) error {
	hs := &http.Server{Handler: s}
	return hs.Serve(l)
}

func (s *tunnelServer) listenAndServe(addr string) error {
	server := &http.Server{Addr: addr, Handler: s}
	return server.ListenAndServe()
}

func (s *tunnelServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	target := req.Header.Get(upstreamHeaderName)
	if len(target) == 0 {
		http.Error(w, "missing target header", http.StatusBadRequest)
		return
	}

	conn, err := s.hijackConnection(w, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.handleConnection(conn, target)
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

func (s *tunnelServer) handleConnection(conn net.Conn, target string) {
	defer conn.Close()
	dst, err := net.DialTimeout("tcp", target, s.timeout)
	if err != nil {
		slog.Error("Unable to dial upstream", "addr", target, "err", err)
		return
	}
	slog.Info("Dialed upstream", "addr", target)
	defer dst.Close()
	pipe(conn, dst)
}

/*
	func connectViaCloudRun(ts oauth2.TokenSource, serviceUrl string) dialFunc {
		return func(network, addr string) (io.ReadWriteCloser, error) {
			u, err := url.Parse(serviceUrl)
			if err != nil {
				return nil, err
			}
			return connect(http.DefaultClient, ts, u, addr)
		}
	}

	func connectViaIAP(ts oauth2.TokenSource, instance string, port int, project, zone string) dialFunc {
		if port == 0 {
			port = DefaultServerPort
		}

		return func(network, addr string) (io.ReadWriteCloser, error) {
			u, err := url.Parse("http://localhost")
			if err != nil {
				return nil, err
			}

			opts := []iap.DialOption{
				iap.WithProject(project),
				iap.WithInstance(instance, zone, "nic0"),
				iap.WithPort(fmt.Sprintf("%d", port)),
				iap.WithTokenSource(&ts),
			}

			clt := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						return iap.Dial(ctx, opts...)
					},
				},
			}

			return connect(clt, ts, u, addr)
		}
	}
*/

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
