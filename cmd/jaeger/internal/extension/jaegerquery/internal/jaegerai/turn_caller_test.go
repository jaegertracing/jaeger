// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/client"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

// testAuthData is what an authenticator leaves in client.Info.Auth: a bag of
// attributes about the caller it resolved.
type testAuthData map[string]any

func (d testAuthData) GetAttribute(name string) any { return d[name] }

func (d testAuthData) GetAttributeNames() []string {
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	return names
}

// chatRequestContext builds a context the way the query server's middleware
// leaves a chat request's: every identity carrier populated.
func chatRequestContext(t *testing.T) context.Context {
	t.Helper()
	ctx := client.NewContext(context.Background(), client.Info{
		Addr: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 51000},
		Auth: testAuthData{"enduser.id": "alice"},
	})
	ctx = bearertoken.ContextWithBearerToken(ctx, "tok-123")
	return headerforwarding.ContextWithCaptured(ctx, []headerforwarding.CapturedHeader{{
		Header: &headerforwarding.ForwardedHeader{HTTPName: "x-user"},
		Value:  "alice",
	}})
}

// sidecarRequestContext builds a context the way a turn-scoped MCP request
// arrives from the sidecar: confighttp has stamped the sidecar's own address
// and nothing identifies a person.
func sidecarRequestContext() context.Context {
	return client.NewContext(context.Background(), client.Info{
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40000},
	})
}

// sidecarRequestContextWithOwnIdentity is a sidecar request that, unlike any
// real one, carries a bearer token and a forwarded header of its own, so a test
// can tell replacement from preservation.
func sidecarRequestContextWithOwnIdentity() context.Context {
	ctx := bearertoken.ContextWithBearerToken(sidecarRequestContext(), "sidecar-token")
	return headerforwarding.ContextWithCaptured(ctx, []headerforwarding.CapturedHeader{{
		Header: &headerforwarding.ForwardedHeader{HTTPName: "x-user"},
		Value:  "sidecar",
	}})
}

func TestTurnCallerInstallsChatIdentityOnToolRequest(t *testing.T) {
	caller := captureTurnCaller(chatRequestContext(t))

	ctx := caller.contextFor(sidecarRequestContextWithOwnIdentity())

	info := client.FromContext(ctx)
	require.NotNil(t, info.Auth, "the chat request's authenticator result reaches the tool call")
	assert.Equal(t, "alice", info.Auth.GetAttribute("enduser.id"))
	assert.Equal(t, "10.0.0.7:51000", info.Addr.String(), "the client of the turn is the person, not the sidecar")

	token, ok := bearertoken.GetBearerToken(ctx)
	require.True(t, ok)
	assert.Equal(t, "tok-123", token)

	forwarded := headerforwarding.CapturedFromContext(ctx)
	require.Len(t, forwarded, 1)
	assert.Equal(t, "alice", forwarded[0].Value, "the chat request's forwarded headers replace the request's own")
}

// A chat request that carried no bearer token or forwarded headers must not
// hand the tool call empty ones: a storage client that sees a token key with an
// empty value behaves differently from one that sees none. What the request
// carried stays as it was.
func TestTurnCallerLeavesAbsentCarriersAsTheyWere(t *testing.T) {
	caller := captureTurnCaller(context.Background())

	t.Run("request without its own identity", func(t *testing.T) {
		ctx := caller.contextFor(sidecarRequestContext())
		_, ok := bearertoken.GetBearerToken(ctx)
		assert.False(t, ok, "no bearer token is installed when the chat request had none")
		assert.Nil(t, headerforwarding.CapturedFromContext(ctx))
	})

	t.Run("request with its own identity", func(t *testing.T) {
		ctx := caller.contextFor(sidecarRequestContextWithOwnIdentity())
		token, ok := bearertoken.GetBearerToken(ctx)
		require.True(t, ok)
		assert.Equal(t, "sidecar-token", token, "the request's own token is kept when the chat request had none")
		forwarded := headerforwarding.CapturedFromContext(ctx)
		require.Len(t, forwarded, 1)
		assert.Equal(t, "sidecar", forwarded[0].Value)
	})
}
