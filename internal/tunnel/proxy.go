package tunnel

import (
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"tailscale.com/net/proxymux"
	"time"
)

func StartProxy(ctx context.Context, addr string, c ProxyConfig) error {
	targets, err := c.createProxyUpstreams(ctx)
	if err != nil {
		return err
	}

	p := &httpProxy{
		targets: targets,
		dialer:  NewDefaultDialer(c.Timeout),
	}

	s := &socks5Proxy{
		targets: targets,
		dialer:  NewDefaultDialer(c.Timeout),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("Listening on %s", addr))

	socksListener, httpListener := proxymux.SplitSOCKSAndHTTP(ln)

	g := new(errgroup.Group)
	g.Go(func() error { return s.serve(socksListener) })
	g.Go(func() error { return p.serve(httpListener) })

	return g.Wait()
}

type ProxyConfig struct {
	Rules   []Rule        `yaml:"rules"`
	Timeout time.Duration `yaml:"dial_timeout"`
}

type Rule struct {
	Tunnel    Tunnel   `yaml:"tunnel"`
	Upstreams []string `yaml:"upstreams"`
}

type Tunnel struct {
	Instance   string `yaml:"instance"`
	Port       int    `yaml:"port"`
	Project    string `yaml:"project"`
	Zone       string `yaml:"zone"`
	ServiceUrl string `yaml:"service_url"`
}

func (c ProxyConfig) createProxyUpstreams(ctx context.Context) ([]proxyUpstream, error) {
	var targets []proxyUpstream

	for _, rule := range c.Rules {
		t := rule.Tunnel
		if t.ServiceUrl != "" {
			u, err := url.Parse(t.ServiceUrl)
			if err != nil {
				return nil, err
			}

			ts, err := findTokenSource(ctx, t.ServiceUrl)
			if err != nil {
				return nil, err
			}

			dialer := NewCloudRunRemoteDialer(ts, u)

			if len(rule.Upstreams) == 0 {
				targets = append(targets, newProxyUpstream("*", dialer))
				continue
			}

			for _, upstream := range rule.Upstreams {
				targets = append(targets, newProxyUpstream(upstream, dialer))
			}
		}

		if t.Instance != "" {
			ts, err := google.DefaultTokenSource(ctx)
			if err != nil {
				return nil, err
			}

			dialer := NewIAPRemoteDialer(ts, t.Instance, t.Port, t.Project, t.Zone)

			if len(rule.Upstreams) == 0 {
				targets = append(targets, newProxyUpstream("*", dialer))
				continue
			}

			for _, upstream := range rule.Upstreams {
				targets = append(targets, newProxyUpstream(upstream, dialer))
			}
		}
	}

	return targets, nil
}

func newProxyUpstream(upstream string, dialer Dialer) proxyUpstream {
	pt := proxyUpstream{
		upstream: upstream,
		dialer:   dialer,
	}

	if prefix, err := netip.ParsePrefix(upstream); err == nil {
		pt.prefix = &prefix
	}

	return pt
}

type proxyUpstream struct {
	upstream string
	prefix   *netip.Prefix
	dialer   Dialer
}

func (u proxyUpstream) matches(candidate string) bool {
	if u.upstream == "*" {
		return true
	}

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

type proxyUpstreams []proxyUpstream

func (p proxyUpstreams) getDialer(target string, local Dialer) (string, Dialer) {
	for _, u := range p {
		if u.matches(target) {
			return "remote", u.dialer
		}
	}

	return "local", local
}