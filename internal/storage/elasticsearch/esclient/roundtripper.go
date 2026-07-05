// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// customHeadersRoundTripper applies statically configured headers to every
// outbound request. It must run before any HTTP authenticator so that signers
// (such as sigv4auth) include these headers in the request signature.
// Headers already present on the request (e.g. set per-request by the
// header_forwarding middleware) take precedence over the static values.
// Note: The ES clients never set req.Host (Go derives it from URL.Host), so
// there's no per-request value to preserve. A configured Host is intentionally
// authoritative, which is required for proxies like aws-sigv4-proxy.
type customHeadersRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *customHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for key, value := range t.headers {
		if strings.EqualFold(key, "Host") {
			req.Host = value
			continue
		}
		if len(req.Header.Values(key)) == 0 {
			req.Header.Set(key, value)
		}
	}
	return t.base.RoundTrip(req)
}

// getBodyFixRoundTripper ensures req.GetBody is populated when req.Body is set.
// The olivere/elastic v7 client sets req.Body directly without setting GetBody,
// which breaks HTTP authenticators (like sigv4auth) that rely on GetBody to hash
// the request payload for signing.
type getBodyFixRoundTripper struct {
	base http.RoundTripper
}

func (t *getBodyFixRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.GetBody == nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return t.base.RoundTrip(req)
}

// GetHTTPRoundTripper returns configured http.RoundTripper with optional HTTP authenticator.
// Pass nil for httpAuth if authentication is not required.
func GetHTTPRoundTripper(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (http.RoundTripper, error) {
	// Configure base transport.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	// Configure TLS.
	if c.TLS.Insecure {
		// #nosec G402
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		tlsConfig, err := c.TLS.LoadTLSConfig(ctx)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}

	// Initialize authentication methods.
	var authMethods []auth.Method
	// API Key Authentication
	if c.Authentication.APIKeyAuth.HasValue() {
		apiKeyAuth := c.Authentication.APIKeyAuth.Get()
		ak, err := initAPIKeyAuth(apiKeyAuth, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize API key authentication: %w", err)
		}
		if ak != nil {
			authMethods = append(authMethods, *ak)
		}
	}

	// Bearer Token Authentication
	if c.Authentication.BearerTokenAuth.HasValue() {
		bearerAuth := c.Authentication.BearerTokenAuth.Get()
		ba, err := initBearerAuth(bearerAuth, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize bearer authentication: %w", err)
		}
		if ba != nil {
			authMethods = append(authMethods, *ba)
		}
	}

	// Basic Authentication
	if c.Authentication.BasicAuthentication.HasValue() {
		basicAuth := c.Authentication.BasicAuthentication.Get()
		ba, err := initBasicAuth(basicAuth, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize basic authentication: %w", err)
		}
		if ba != nil {
			authMethods = append(authMethods, *ba)
		}
	}

	// Wrap with authentication layer.
	var roundTripper http.RoundTripper = transport
	if len(authMethods) > 0 {
		roundTripper = &auth.RoundTripper{
			Transport: transport,
			Auths:     authMethods,
		}
	}

	// Apply HTTP authenticator extension if configured (e.g., SigV4).
	// The getBodyFixRoundTripper must wrap OUTSIDE the authenticator so it
	// runs first: authenticators like sigv4auth hash the payload via
	// req.GetBody at the start of their RoundTrip, before delegating to the
	// inner transport. The upstream HTTP client (olivere/elastic) sets
	// req.Body without setting GetBody, so GetBody must be populated before
	// the authenticator sees the request, not after.
	if httpAuth != nil {
		wrappedRT, err := httpAuth.RoundTripper(roundTripper)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap round tripper with HTTP authenticator: %w", err)
		}
		roundTripper = &getBodyFixRoundTripper{base: wrappedRT}
	}

	// Applied at the transport level so that both the olivere v7 and
	// go-elasticsearch v8 clients, as well as sniffing and health-check
	// requests, all carry the configured headers. Wrapped outside the
	// authenticator so the signer includes these headers in the signature.
	if len(c.CustomHeaders) > 0 {
		roundTripper = &customHeadersRoundTripper{base: roundTripper, headers: c.CustomHeaders}
	}

	return roundTripper, nil
}
