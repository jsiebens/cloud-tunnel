package proxy

import (
	"context"
	"fmt"
	"github.com/jsiebens/cloud-tunnel/pkg/iap"
	"github.com/jsiebens/cloud-tunnel/pkg/remotedialer"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/impersonate"
	"net"
	"net/url"
	"time"
)

const (
	DefaultTimeout     = 5 * time.Second
	DefaultServerPort  = 7654
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
)

func defaultRemoteDialer(mux bool, ts oauth2.TokenSource, url *url.URL) remotedialer.Dialer {
	dialer := remotedialer.Dialer(&net.Dialer{})
	if mux {
		dialer = remotedialer.Muxed(dialer)
	}

	return remotedialer.RemoteDialer(url, ts, dialer)
}

func iapRemoteDialer(mux bool, ts oauth2.TokenSource, instance string, port int, project, zone string) remotedialer.Dialer {
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

	dialer := remotedialer.IAPDialer(ts, opts)
	if mux {
		dialer = remotedialer.Muxed(dialer)
	}

	return remotedialer.RemoteDialer(u, ts, dialer)
}

func tokenSource(ctx context.Context, serviceAccount string) (oauth2.TokenSource, error) {
	if serviceAccount != "" {
		return impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: serviceAccount,
			Scopes:          []string{cloudPlatformScope},
		})
	}

	return google.DefaultTokenSource(ctx)
}

func idTokenSource(ctx context.Context, audience string, serviceAccount string) (oauth2.TokenSource, error) {
	if serviceAccount != "" {
		tokenSource, err := impersonate.IDTokenSource(ctx, impersonate.IDTokenConfig{
			Audience:        audience,
			TargetPrincipal: serviceAccount,
			IncludeEmail:    true,
		})
		if err != nil {
			tokenSource, err = impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
				TargetPrincipal: serviceAccount,
				Scopes:          []string{cloudPlatformScope},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get default token source: %w", err)
			}

			tokenSource = &idTokenFromDefaultTokenSource{ts: tokenSource}
		}

		return oauth2.ReuseTokenSource(nil, tokenSource), nil
	}

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
