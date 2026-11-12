// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension/extensionauth"
)

// authenticator applies the extension named by ai.auth to the AI surfaces of
// the query port, with the inputs the collector's confighttp gives one for a
// whole server: the request headers plus the query parameters listed in
// request_params. Register builds one only when the ai.auth block is present.
type authenticator struct {
	server        extensionauth.Server
	requestParams []string
}

// newAuthenticator resolves cfg against the host's extensions. An extension
// that is not loaded, or is not a server authenticator, is a configuration
// error that stops startup.
func newAuthenticator(ctx context.Context, cfg *confighttp.AuthConfig, host component.Host) (*authenticator, error) {
	server, err := cfg.Config.GetServerAuthenticator(ctx, host.GetExtensions())
	if err != nil {
		return nil, fmt.Errorf("ai.auth: %w", err)
	}
	return &authenticator{server: server, requestParams: cfg.RequestParameters}, nil
}

// authenticate runs the extension on r and returns the context it produced,
// which carries whatever the extension recorded about the caller.
func (a *authenticator) authenticate(r *http.Request) (context.Context, error) {
	sources := r.Header
	if len(a.requestParams) > 0 {
		sources = r.Header.Clone()
		query := r.URL.Query()
		for _, param := range a.requestParams {
			if values, ok := query[param]; ok {
				sources[param] = values
			}
		}
	}
	return a.server.Authenticate(r.Context(), sources)
}

// admits reports whether the extension accepts r, for a surface that hides a
// feature from a rejected caller instead of refusing the request.
func (a *authenticator) admits(r *http.Request) bool {
	_, err := a.authenticate(r)
	return err == nil
}

// wrap answers a rejected request with 401 before next sees it, and hands an
// admitted request to next with the authenticated context.
func (a *authenticator) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := a.authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx)) //nolint:contextcheck // authenticate derives ctx from r.Context().
	})
}
