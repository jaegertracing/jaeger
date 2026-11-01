// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"

	"go.opentelemetry.io/collector/client"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

// turnCaller is the identity that the chat request carried, captured when the
// turn opens and installed on every request the agent makes to the turn-scoped
// MCP endpoint. The agent's own requests name nobody: they arrive from the
// sidecar carrying the route id and no identity headers, so without this every
// chat-driven query would reach the query service as an anonymous caller, and
// an interceptor that filters or masks by caller would treat the person's own
// question as nobody's.
//
// The three fields are the carriers that the query server's own HTTP middleware
// reads identity from on a direct request: the collector's client.Info, which
// confighttp sets on every request and which an authenticator fills with the
// caller it resolved; the bearer token that bearer_token_propagation forwards to
// storage; and the headers that header_forwarding captures. The tenant is not
// among them: with tenancy enabled the tenant middleware rejects the sidecar's
// request, which carries no tenant header, before it reaches this endpoint.
type turnCaller struct {
	client      client.Info
	bearerToken string
	forwarded   []headerforwarding.CapturedHeader
}

// captureTurnCaller reads the caller out of the chat request's context.
func captureTurnCaller(ctx context.Context) turnCaller {
	token, _ := bearertoken.GetBearerToken(ctx)
	return turnCaller{
		client:      client.FromContext(ctx),
		bearerToken: token,
		forwarded:   headerforwarding.CapturedFromContext(ctx),
	}
}

// contextFor returns ctx, a turn-scoped MCP request's context, with the chat
// caller installed. client.Info is replaced outright: confighttp has already
// stamped the sidecar's own address on this request, and the client of a turn
// is the person who asked, not the process relaying their question. The bearer
// token and the forwarded headers replace whatever the request carried when the
// chat request had them, and are left as they were when it did not, because
// both setters leave the context alone for an empty value.
func (c turnCaller) contextFor(ctx context.Context) context.Context {
	ctx = client.NewContext(ctx, c.client)
	ctx = bearertoken.ContextWithBearerToken(ctx, c.bearerToken)
	return headerforwarding.ContextWithCaptured(ctx, c.forwarded)
}
