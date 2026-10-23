// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// headerAuthenticator is an extensionauth.Server that admits a request carrying
// the configured header, or the configured query parameter once the
// authenticator has copied it into the sources, and records the admitted user
// on the context.
type headerAuthenticator struct {
	component.StartFunc
	component.ShutdownFunc
	header string
	param  string
}

type authenticatedUserKey struct{}

func (a *headerAuthenticator) Authenticate(ctx context.Context, sources map[string][]string) (context.Context, error) {
	for _, key := range []string{a.header, a.param} {
		if values := sources[key]; len(values) > 0 {
			return context.WithValue(ctx, authenticatedUserKey{}, values[0]), nil
		}
	}
	return ctx, errors.New("no user")
}

// plainExtension is a component that is not an authenticator.
type plainExtension struct {
	component.StartFunc
	component.ShutdownFunc
}

// extensionsHost is a component.Host whose extensions the test chooses.
type extensionsHost struct {
	component.Host
	extensions map[component.ID]component.Component
}

func (h *extensionsHost) GetExtensions() map[component.ID]component.Component {
	return h.extensions
}

func TestAuthenticator(t *testing.T) {
	authID := component.MustNewID("testauth")
	host := &extensionsHost{extensions: map[component.ID]component.Component{
		authID: &headerAuthenticator{header: "X-Test-User", param: "user"},
	}}
	auth, err := newAuthenticator(context.Background(), &confighttp.AuthConfig{
		Config:            configauth.Config{AuthenticatorID: authID},
		RequestParameters: []string{"user"},
	}, host)
	require.NoError(t, err)

	var seenUser any
	var seenHeader http.Header
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUser = r.Context().Value(authenticatedUserKey{})
		seenHeader = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	})
	handler := auth.wrap(next)

	request := func(target string, header http.Header) *http.Request {
		req := httptest.NewRequest(http.MethodPost, target, http.NoBody)
		req.Header = header
		return req
	}
	serve := func(target string, header http.Header) *httptest.ResponseRecorder {
		seenUser, seenHeader = nil, nil
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, request(target, header))
		return rr
	}

	t.Run("rejected request never reaches the handler", func(t *testing.T) {
		rr := serve("/mcp", http.Header{})
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "no user")
		assert.Nil(t, seenHeader)
		assert.False(t, auth.admits(request("/mcp", http.Header{})))
	})

	t.Run("authenticated context reaches the handler", func(t *testing.T) {
		rr := serve("/mcp", http.Header{"X-Test-User": {"alice"}})
		assert.Equal(t, http.StatusNoContent, rr.Code)
		assert.Equal(t, "alice", seenUser)
		assert.True(t, auth.admits(request("/mcp", http.Header{"X-Test-User": {"alice"}})))
	})

	t.Run("a listed query parameter counts, without touching the headers", func(t *testing.T) {
		rr := serve("/mcp?user=bob", http.Header{"Accept": {"text/event-stream"}})
		assert.Equal(t, http.StatusNoContent, rr.Code)
		assert.Equal(t, "bob", seenUser)
		assert.Equal(t, http.Header{"Accept": {"text/event-stream"}}, seenHeader)
	})

	t.Run("a listed query parameter overrides a header of the same name", func(t *testing.T) {
		sameName, err := newAuthenticator(context.Background(), &confighttp.AuthConfig{
			Config:            configauth.Config{AuthenticatorID: authID},
			RequestParameters: []string{"X-Test-User"},
		}, host)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		sameName.wrap(next).ServeHTTP(rr, request("/mcp?X-Test-User=bob", http.Header{"X-Test-User": {"alice"}}))
		assert.Equal(t, http.StatusNoContent, rr.Code)
		assert.Equal(t, "bob", seenUser)
	})

	t.Run("no ai.auth block yields no authenticator", func(t *testing.T) {
		none, err := newAuthenticator(context.Background(), nil, host)
		require.NoError(t, err)
		assert.Nil(t, none)
	})
}

func TestRegisterRejectsUnusableAuthenticator(t *testing.T) {
	notAnAuthenticator := component.MustNewID("plain")
	telset := telemetry.NoopSettings()
	telset.Logger = zaptest.NewLogger(t)
	telset.Host = &extensionsHost{extensions: map[component.ID]component.Component{
		notAnAuthenticator: plainExtension{},
	}}
	cases := []struct {
		name string
		id   component.ID
		want string
	}{
		{name: "not loaded", id: component.MustNewID("missing"), want: "authenticator not found"},
		{name: "not a server authenticator", id: notAnAuthenticator, want: "not a server authenticator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Auth = configoptional.Some(confighttp.AuthConfig{Config: configauth.Config{AuthenticatorID: tc.id}})

			routes, err := Register(context.Background(), http.NewServeMux(), RegisterParams{Config: &cfg, Telemetry: telset})
			require.ErrorContains(t, err, "ai.auth")
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, routes)
		})
	}
}

// Without ai.auth, Routes admits everyone, so the query server needs no branch
// for a deployment that left the setting out.
func TestRoutesAdmitsEveryoneWithoutAuth(t *testing.T) {
	routes := &Routes{}
	require.True(t, routes.Admits(httptest.NewRequest(http.MethodGet, "/", http.NoBody)))
}
