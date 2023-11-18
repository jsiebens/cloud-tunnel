package tunnel

import (
	"context"
	"fmt"
	"github.com/hashicorp/yamux"
	"log/slog"
	"net/http"
)

type HttpProxyConfig struct {
	ServiceUrl string
	Instance   string
	Port       int
	Project    string
	Zone       string
}

func StartHttpProxy(ctx context.Context, addr string, c HttpProxyConfig) error {
	if c.ServiceUrl != "" {
		ts, err := findTokenSource(ctx, c.ServiceUrl)
		if err != nil {
			return err
		}

		p := &httpProxy{
			addr: addr,
			dial: connectViaCloudRun(ts, c.ServiceUrl),
		}

		return p.start()
	}

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

	p := &httpProxy{
		addr: addr,
		dial: connectViaMux(session),
	}

	go p.start()

	<-session.CloseChan()
	return fmt.Errorf("session closed")
}

type httpProxy struct {
	addr string
	dial dialFunc
}

func (hp *httpProxy) start() error {
	slog.Info(fmt.Sprintf("Listening on %s", hp.addr))
	return http.ListenAndServe(hp.addr, hp)
}

func (hp *httpProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		hp.connect(w, req)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (hp *httpProxy) connect(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	conn, err := hp.dial("tcp", req.Host)
	if err != nil {
		slog.Error("Error dialing upstream", "addr", req.Host, "err", err)
		http.Error(w, fmt.Sprintf("Unable to dial %s, error: %s", req.Host, err.Error()), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)

	slog.Info("Dialed remote upstream", "addr", req.Host)

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
