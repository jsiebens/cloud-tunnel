package tunnel

import (
	"context"
	"fmt"
	"github.com/jsiebens/cloud-tunnel/internal/iap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	DefaultTimeout          = 5 * time.Second
	DefaultServerPort       = 7654
	AuthorizationHeaderName = "Authorization"
	UpstreamHeaderName      = "X-Cloud-Tunnel-Upstream"
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func NewDefaultDialer(timeout time.Duration) Dialer {
	return &net.Dialer{Timeout: timeout}
}

type RemoteDialer struct {
	url *url.URL
	ts  oauth2.TokenSource
	clt *http.Client
}

func (r *RemoteDialer) DialContext(ctx context.Context, _, addr string) (net.Conn, error) {
	return connect(ctx, r.clt, r.ts, r.url, addr)
}

func NewIAPRemoteDialer(ts oauth2.TokenSource, instance string, port int, project, zone string) Dialer {
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

	clt := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return iap.Dial(ctx, ts, opts)
			},
		},
	}

	return &RemoteDialer{u, ts, clt}
}

func NewCloudRunRemoteDialer(ts oauth2.TokenSource, url *url.URL) Dialer {
	return &RemoteDialer{url, ts, http.DefaultClient}
}

func findTokenSource(ctx context.Context, audience string) (oauth2.TokenSource, error) {
	tokenSource, err := idtoken.NewTokenSource(ctx, audience)

	if err != nil {
		tokenSource, err = google.DefaultTokenSource(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get default token source: %w", err)
		}

		tokenSource = &idTokenFromDefaultTokenSource{ts: tokenSource}
	}

	return oauth2.ReuseTokenSource(nil, tokenSource), nil
}

type idTokenFromDefaultTokenSource struct {
	ts oauth2.TokenSource
}

func (s *idTokenFromDefaultTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.ts.Token()
	if err != nil {
		return nil, err
	}

	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("missing id_token")
	}

	return &oauth2.Token{
		AccessToken: idToken,
		Expiry:      token.Expiry,
	}, nil
}

func connect(ctx context.Context, clt *http.Client, ts oauth2.TokenSource, url *url.URL, upstream string) (net.Conn, error) {
	req := &http.Request{
		Method: "GET",
		URL:    url,
		Header: http.Header{
			"Upgrade":          []string{"websocket"},
			"Connection":       []string{"upgrade"},
			UpstreamHeaderName: []string{upstream},
		},
	}

	if ts != nil {
		token, err := ts.Token()
		if err != nil {
			return nil, err
		}
		req.Header.Set(AuthorizationHeaderName, "Bearer "+token.AccessToken)
	}

	resp, err := clt.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("invalid response: %s", resp.Status)
	}

	return rwcConn{rwc: resp.Body.(io.ReadWriteCloser), addr: upstream}, nil
}
