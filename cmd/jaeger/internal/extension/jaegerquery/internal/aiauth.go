// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension/extensionauth"
)

// aiAuthenticator applies the authenticator extension named by ai.auth to the
// AI surfaces of the query port, the way the collector's confighttp applies one
// to a whole server: the request headers plus the query parameters listed in
// request_params go to the extension. A nil *aiAuthenticator admits everyone,
// so callers need no branch for a deployment without the setting.
type aiAuthenticator struct {
	server        extensionauth.Server
	requestParams []string
}

// newAIAuthenticator resolves cfg against the host's extensions. A nil cfg
// returns a nil authenticator. An extension that is not loaded, or is not a
// server authenticator, is a configuration error that stops startup.
func newAIAuthenticator(ctx context.Context, cfg *confighttp.AuthConfig, host component.Host) (*aiAuthenticator, error) {
	if cfg == nil {
		return nil, nil
	}
	var extensions map[component.ID]component.Component
	if host != nil {
		extensions = host.GetExtensions()
	}
	server, err := cfg.Config.GetServerAuthenticator(ctx, extensions)
	if err != nil {
		return nil, fmt.Errorf("ai.auth: %w", err)
	}
	return &aiAuthenticator{server: server, requestParams: cfg.RequestParameters}, nil
}

// Authenticate runs the extension on r and returns the context it produced,
// which carries whatever the extension recorded about the caller.
func (a *aiAuthenticator) Authenticate(r *http.Request) (context.Context, error) {
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

// Admits reports whether the extension accepts r, for a surface that hides a
// feature from a rejected caller instead of refusing the request.
func (a *aiAuthenticator) Admits(r *http.Request) bool {
	if a == nil {
		return true
	}
	_, err := a.Authenticate(r)
	return err == nil
}

// GateCapabilities returns a provider that reports the AI assistant only to a
// caller the extension admits, so the UI shows no chat panel to anyone else.
func (a *aiAuthenticator) GateCapabilities(next BackendCapabilityProvider) BackendCapabilityProvider {
	if a == nil || next == nil {
		return next
	}
	return func(r *http.Request) BackendCapabilities {
		caps := next(r)
		caps.AIAssistant = caps.AIAssistant && a.Admits(r)
		return caps
	}
}

// Wrap answers a rejected request with 401 before next sees it, and hands an
// admitted request to next with the authenticated context.
func (a *aiAuthenticator) Wrap(next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := a.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx)) //nolint:contextcheck // Authenticate derives ctx from r.Context().
	})
}
