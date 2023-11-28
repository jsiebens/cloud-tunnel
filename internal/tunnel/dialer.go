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

type Dialer interface {
	Dial(network, addr string) (io.ReadWriteCloser, error)
}

type DefaultDialer struct {
	timeout time.Duration
}

func (d *DefaultDialer) Dial(network, addr string) (io.ReadWriteCloser, error) {
	return net.DialTimeout(network, addr, d.timeout)
}

type RemoteDialer struct {
	url *url.URL
	ts  oauth2.TokenSource
	clt *http.Client
}

func (r *RemoteDialer) Dial(_, addr string) (io.ReadWriteCloser, error) {
	return connect(r.clt, r.ts, r.url, addr)
}

func NewDefaultDialer(timeout time.Duration) Dialer {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &DefaultDialer{timeout}
}

func NewIAPRemoteDialer(ts oauth2.TokenSource, instance string, port int, project, zone string) Dialer {
	if port == 0 {
		port = DefaultServerPort
	}

	u, _ := url.Parse("http://localhost")

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

func connect(clt *http.Client, ts oauth2.TokenSource, url *url.URL, upstream string) (io.ReadWriteCloser, error) {
	req := &http.Request{
		Method: "GET",
		URL:    url,
		Header: http.Header{
			"Upgrade":          []string{"websocket"},
			"Connection":       []string{"upgrade"},
			upstreamHeaderName: []string{upstream},
		},
	}

	if ts != nil {
		token, err := ts.Token()
		if err != nil {
			return nil, err
		}
		req.Header.Set(authorizationHeaderName, "Bearer "+token.AccessToken)
	}

	resp, err := clt.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("invalid response: %s", resp.Status)
	}

	return resp.Body.(io.ReadWriteCloser), nil
}
