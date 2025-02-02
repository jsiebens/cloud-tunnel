package remotedialer

import (
	"context"
	"fmt"
	"github.com/hashicorp/yamux"
	"github.com/jsiebens/cloud-tunnel/pkg/iap"
	"golang.org/x/oauth2"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
)

const (
	AuthorizationHeaderName = "Authorization"
	UpstreamHeaderName      = "X-Cloud-Tunnel-Upstream"
	DefaultServerPort       = 7654
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func RemoteDialer(ts oauth2.TokenSource, url *url.URL, mux bool) Dialer {
	dialer := Dialer(&net.Dialer{})
	if mux {
		dialer = muxed(dialer)
	}

	return &remoteDialer{url: url, ts: ts, dialer: dialer}
}

func IAPRemoteDialer(ts oauth2.TokenSource, instance string, port int, project, zone string, mux bool) Dialer {
	if port == 0 {
		port = DefaultServerPort
	}

	u, _ := url.Parse("http://unused")

	opts := iap.DialOptions{
		Project:  project,
		Zone:     zone,
		Instance: instance,
		Port:     port,
	}

	dialer := Dialer(&iapDialer{ts, opts})
	if mux {
		dialer = muxed(dialer)
	}

	return &remoteDialer{url: u, ts: ts, dialer: dialer}
}

type remoteDialer struct {
	url    *url.URL
	ts     oauth2.TokenSource
	dialer Dialer
}

func (r *remoteDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network '%s'", network)
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	if r.dialer != nil {
		tr.DialContext = r.dialer.DialContext
	}
	defer tr.CloseIdleConnections()

	req := &http.Request{
		Method: "GET",
		URL:    r.url,
		Header: http.Header{
			"Upgrade":          []string{"websocket"},
			"Connection":       []string{"upgrade"},
			UpstreamHeaderName: []string{addr},
		},
	}

	if r.ts != nil {
		token, err := r.ts.Token()
		if err != nil {
			return nil, err
		}
		req.Header.Set(AuthorizationHeaderName, "Bearer "+token.AccessToken)
	}

	resp, err := tr.RoundTrip(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("invalid response: %s", resp.Status)
	}

	return rwcConn{rwc: resp.Body.(io.ReadWriteCloser), addr: addr}, nil
}

type iapDialer struct {
	ts   oauth2.TokenSource
	opts iap.DialOptions
}

func (i *iapDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	return iap.Dial(ctx, i.ts, i.opts)
}

func muxed(dialer Dialer) Dialer {
	return &muxedDialer{dialer: dialer, sessions: make(map[string]*yamux.Session)}
}

type muxedDialer struct {
	sync.RWMutex
	dialer   Dialer
	sessions map[string]*yamux.Session
}

func (i *muxedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	session, err := i.getSession(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return session.Open()
}

func (i *muxedDialer) getSession(ctx context.Context, network, addr string) (*yamux.Session, error) {
	k := fmt.Sprintf("%s|%s", network, addr)

	i.RLock()
	{
		session := i.sessions[k]
		if session != nil && !session.IsClosed() {
			i.RUnlock()
			return session, nil
		}
	}
	i.RUnlock()

	i.Lock()
	defer i.Unlock()

	session := i.sessions[k]
	if session != nil && !session.IsClosed() {
		return session, nil
	}

	conn, err := i.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	if i.sessions[k], err = yamux.Client(conn, nil); err != nil {
		return nil, err
	}

	return i.sessions[k], nil
}
