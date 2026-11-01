// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"

	"go.opentelemetry.io/collector/client"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

// turnCaller is the chat request's caller, installed on every request the agent
// makes to the turn-scoped MCP endpoint so that tool calls run as the person who
// asked rather than as the anonymous sidecar. The fields are the carriers the
// query server's middleware reads identity from: client.Info, the propagated
// bearer token and the header_forwarding captures. The tenant is absent because
// the tenant middleware rejects the sidecar's header-less request first.
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

// contextFor returns ctx with the chat caller installed. client.Info replaces
// the sidecar's own, which confighttp stamped on the request; the other two
// setters are no-ops for an empty value, so an absent carrier leaves ctx as is.
func (c turnCaller) contextFor(ctx context.Context) context.Context {
	ctx = client.NewContext(ctx, c.client)
	ctx = bearertoken.ContextWithBearerToken(ctx, c.bearerToken)
	return headerforwarding.ContextWithCaptured(ctx, c.forwarded)
}
