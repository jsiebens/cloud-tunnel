package tunnel

import (
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"
)

type HttpProxyConfig struct {
	Rules []Rule `yaml:"rules"`
}

type Rule struct {
	Tunnel    TunnelConfig `yaml:"tunnel"`
	Upstreams []string     `yaml:"upstreams"`
}

type TunnelConfig struct {
	Instance   string `yaml:"instance"`
	Port       int    `yaml:"port"`
	Project    string `yaml:"project"`
	Zone       string `yaml:"zone"`
	ServiceUrl string `yaml:"service_url"`
}

func StartHttpProxy(ctx context.Context, addr string, c HttpProxyConfig) error {
	var targets []proxyTarget

	for _, rule := range c.Rules {
		t := rule.Tunnel
		if t.ServiceUrl != "" {
			ts, err := findTokenSource(ctx, t.ServiceUrl)
			if err != nil {
				return err
			}

			for _, upstream := range rule.Upstreams {
				u := proxyTarget{
					upstream: upstream,
					dial:     connectViaCloudRun(ts, t.ServiceUrl),
				}

				if prefix, err := netip.ParsePrefix(upstream); err == nil {
					u.prefix = &prefix
				}

				targets = append(targets, u)
			}
		}

		if t.Instance != "" {
			ts, err := google.DefaultTokenSource(ctx)
			if err != nil {
				return err
			}

			for _, upstream := range rule.Upstreams {
				u := proxyTarget{
					upstream: upstream,
					dial:     connectViaIAP(ts, t.Instance, t.Port, t.Project, t.Zone),
				}

				if prefix, err := netip.ParsePrefix(upstream); err == nil {
					u.prefix = &prefix
				}

				targets = append(targets, u)
			}
		}
	}

	p := &httpProxy{
		addr:    addr,
		targets: targets,
	}

	return p.start()
}

type httpProxy struct {
	addr    string
	targets []proxyTarget
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

	mode, dial := hp.getDialer(req.Host)

	conn, err := dial("tcp", req.Host)
	if err != nil {
		slog.Error("Error dialing upstream", "addr", req.Host, "err", err)
		http.Error(w, fmt.Sprintf("Unable to dial %s, error: %s", req.Host, err.Error()), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)

	slog.Info("Dialed upstream", "addr", req.Host, "mode", mode)

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

func (hp *httpProxy) getDialer(target string) (string, dialFunc) {
	for _, u := range hp.targets {
		if u.matches(target) {
			return "remote", u.dial
		}
	}

	return "local", func(network, addr string) (io.ReadWriteCloser, error) {
		return net.DialTimeout(network, addr, time.Second*5)
	}
}

type proxyTarget struct {
	upstream string
	prefix   *netip.Prefix
	dial     dialFunc
}

func (u proxyTarget) matches(candidate string) bool {
	if candidate == u.upstream {
		return true
	}

	host, _, err := net.SplitHostPort(candidate)
	if err != nil {
		return false
	}

	if host == u.upstream {
		return true
	}

	if u.prefix != nil {
		if addr, err := netip.ParseAddr(host); err == nil && u.prefix.Contains(addr) {
			return true
		}
	}

	return false
}
