package tunnel

import (
	"context"
	"fmt"
	"github.com/jsiebens/cloud-tunnel/internal/remotedialer"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type httpProxy struct {
	dialer  remotedialer.Dialer
	targets proxyUpstreams
}

func (hp *httpProxy) serve(ln net.Listener) error {
	return http.Serve(ln, hp)
}

func (hp *httpProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		hp.proxyGet(w, req)
		return
	}

	if req.Method == http.MethodConnect {
		hp.proxyConnect(w, req)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (hp *httpProxy) proxyGet(w http.ResponseWriter, req *http.Request) {
	target, err := url.Parse(req.URL.Scheme + "://" + req.URL.Host)
	if err != nil {
		slog.Error("URL parsing failed", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p := httputil.NewSingleHostReverseProxy(target)
	p.Transport = &http.Transport{
		DialContext: hp.dialContext,
	}

	p.ServeHTTP(w, req)
}

func (hp *httpProxy) proxyConnect(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	conn, err := hp.dialContext(req.Context(), "tcp", req.Host)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to dial %s, error: %s", req.Host, err.Error()), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Unable to hijack connection", http.StatusInternalServerError)
		return
	}

	reqConn, wbuf, err := hj.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to hijack connection %s", err), http.StatusInternalServerError)
		return
	}
	defer reqConn.Close()
	defer wbuf.Flush()

	pipe(conn, reqConn)
}

func (hp *httpProxy) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	mode, dialer := hp.targets.getDialer(addr, hp.dialer)
	conn, err := dialer.DialContext(ctx, network, addr)

	if err != nil {
		slog.Error("Error dialing upstream", "addr", addr, "err", err)
		return conn, err
	}

	slog.Info("Dialed upstream", "addr", addr, "mode", mode)
	return conn, err
}
