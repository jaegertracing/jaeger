// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"net/http"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/client"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// credentialHeaders are bound to a session only when a confighttp authenticator
// accepted the request that opened it, because the session keeps that request's
// client.Info.Auth. Binding them otherwise would refuse a client that refreshes
// its token mid-session, which clients do not recover from.
var credentialHeaders = []string{"Authorization", "X-Forwarded-Access-Token"}

var errSessionCallerMismatch = &jsonrpc.Error{
	Code:    jsonrpc.CodeInvalidRequest,
	Message: "the MCP session was opened with a different tenant or credentials; start a new session",
}

// sessionCallerKey holds the caller-identifying headers of a request. The SDK
// serves every request on an MCP session with the context of the request that
// opened it, so on later requests it holds the opener's headers.
type sessionCallerKey struct{}

// recordSessionCaller stores in each request's context the headers its tenant,
// forwarded headers and authenticated identity were taken from.
func recordSessionCaller(tenancyMgr *tenancy.Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var names []string
		if tenancyMgr.Enabled {
			names = append(names, tenancyMgr.Header)
		}
		for _, captured := range headerforwarding.CapturedFromContext(ctx) {
			names = append(names, captured.Header.HTTPName)
		}
		if client.FromContext(ctx).Auth != nil {
			names = append(names, credentialHeaders...)
		}
		caller := make(http.Header, len(names))
		for _, name := range names {
			caller[http.CanonicalHeaderKey(name)] = slices.Clone(r.Header.Values(name))
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sessionCallerKey{}, caller)))
	})
}

// checkSessionCaller rejects a request whose recorded headers differ from those
// of the request that opened its session, and gives the request its own bearer
// token and client metadata in place of the opener's.
func checkSessionCaller(logger *zap.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			opener, ok := ctx.Value(sessionCallerKey{}).(http.Header)
			if !ok {
				// The session was not opened over HTTP, so it has no caller to check.
				return next(ctx, method, req)
			}
			reject := func(mismatch string) (mcp.Result, error) {
				logger.Info("Rejected an MCP request from a caller other than the session's",
					zap.String("method", method), zap.String("mismatch", mismatch))
				return nil, errSessionCallerMismatch
			}
			var header http.Header
			if extra := req.GetExtra(); extra != nil {
				header = extra.Header
			}
			for name, values := range opener {
				if !slices.Equal(values, header.Values(name)) {
					return reject(name)
				}
			}
			if _, ok := bearertoken.GetBearerToken(ctx); ok {
				// The opener's token cannot be taken out of the context, so a
				// request that carries none is refused rather than served with it.
				token, _ := bearertoken.TokenFromHTTPHeader(header)
				if token == "" {
					return reject("bearer token")
				}
				ctx = bearertoken.ContextWithBearerToken(ctx, token)
			}
			return next(withRequestMetadata(ctx, header), method, req)
		}
	}
}

// withRequestMetadata gives the request the client metadata of its own headers
// instead of the opener's, so a query interceptor that reads the caller from
// client metadata sees who is asking now. confighttp sets the Host entry only
// when include_metadata is on, and RequestExtra does not carry the Host, so the
// opener's Host decides whether to replace the metadata and is kept.
func withRequestMetadata(ctx context.Context, header http.Header) context.Context {
	info := client.FromContext(ctx)
	host := info.Metadata.Get(client.MetadataHostName)
	if len(host) == 0 {
		return ctx
	}
	md := http.Header{client.MetadataHostName: host}
	for name, values := range header {
		md[name] = slices.Clone(values)
	}
	info.Metadata = client.NewMetadata(md)
	return client.NewContext(ctx, info)
}
