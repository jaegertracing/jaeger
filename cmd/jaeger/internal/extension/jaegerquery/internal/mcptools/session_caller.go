// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/client"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// sessionBindingHeader carries a fingerprint of the authenticated caller into
// the request the SDK shows receiving middleware. The handler sets it from the
// authenticator's result and overwrites anything the client sent. It is not
// logged; a mismatch is reported as authenticatedIdentityMismatch.
const sessionBindingHeader = "X-Jaeger-Mcp-Session-Binding"

const authenticatedIdentityMismatch = "authenticated identity"

// volatileSessionHeaders are the ones a client, proxy, or the MCP protocol
// itself changes between the request that opens a session and a later call on
// it. They are not authenticator inputs. confighttp gives an authenticator
// every other header, plus auth.request_params, so those stay in the
// fingerprint when the principal itself is not available.
var volatileSessionHeaders = func() map[string]struct{} {
	names := []string{
		"Accept-Encoding",
		"Accept-Language",
		"Cache-Control",
		"Connection",
		"Content-Encoding",
		"Content-Length",
		"Content-Type",
		"Date",
		"Expect",
		"Host",
		"Keep-Alive",
		"Pragma",
		"Proxy-Connection",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
		"Via",
		"Mcp-Session-Id",
		"Mcp-Protocol-Version",
		"Mcp-Method",
		"Mcp-Name",
		"Last-Event-Id",
		"Traceparent",
		"Tracestate",
		"Baggage",
		"Uber-Trace-Id",
		"X-B3-Traceid",
		"X-B3-Spanid",
		"X-B3-Parentspanid",
		"X-B3-Sampled",
		"X-B3-Flags",
		"X-Request-Id",
		"X-Correlation-Id",
		"Request-Id",
		"X-Cloud-Trace-Context",
		"Sentry-Trace",
	}
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	return out
}()

var errSessionCallerMismatch = &jsonrpc.Error{
	Code:    jsonrpc.CodeInvalidRequest,
	Message: "the MCP session was opened with a different tenant or credentials; start a new session",
}

// sessionCallerKey holds the caller-identifying headers of a request. The SDK
// serves every request on an MCP session with the context of the request that
// opened it, so on later requests it holds the opener's headers.
type sessionCallerKey struct{}

// recordSessionCaller stores in each request's context the headers its tenant,
// forwarded headers and authenticated identity were taken from. Every forwarded
// header is recorded, with no values when the request lacks it, so a later
// request cannot add a header the opener did not send.
//
// Authentication is not those two bearer headers. A confighttp authenticator
// is given every header and the configured query parameters, and it may
// return the caller as client.Info.Auth or only in a context value of its
// own. The session keeps the opener's context either way. When the
// authenticator set a principal, the session is bound to that principal, so a
// cookie, a custom header, or a query parameter is covered without refusing a
// refresh of a header the authenticator did not put in the principal. When
// it did not, the session is bound to the inputs the authenticator actually
// received. Binding bearer headers in the default config would refuse a
// client that refreshes its token, which clients do not recover from.
func recordSessionCaller(tenancyMgr *tenancy.Manager, cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header == nil {
			r.Header = make(http.Header)
		}
		ctx := r.Context()
		var names []string
		if tenancyMgr.Enabled {
			names = append(names, tenancyMgr.Header)
		}
		for i := range cfg.HeaderForwarding {
			names = append(names, cfg.HeaderForwarding[i].HTTPName)
		}
		if binding := sessionBinding(r, cfg); binding != "" {
			r.Header.Set(sessionBindingHeader, binding)
			names = append(names, sessionBindingHeader)
		}
		caller := make(http.Header, len(names))
		for _, name := range names {
			caller[http.CanonicalHeaderKey(name)] = slices.Clone(r.Header.Values(name))
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sessionCallerKey{}, caller)))
	})
}

// sessionBinding is the fingerprint a later request must repeat. Empty means
// the request was not authenticated, so a header the server does not read
// (a refreshed bearer token, in the default config) must not end the session.
func sessionBinding(r *http.Request, cfg Config) string {
	authn := client.FromContext(r.Context()).Auth
	if hasPrincipal(authn) {
		return principalFingerprint(authn)
	}
	// Auth with no attributes, or an authenticator configured in front of the
	// handler that left the caller only in a context value.
	if authn != nil || cfg.Authenticated {
		return authInputFingerprint(r, cfg.AuthRequestParameters)
	}
	return ""
}

func hasPrincipal(authn client.AuthData) bool {
	return authn != nil && len(authn.GetAttributeNames()) > 0
}

func principalFingerprint(authn client.AuthData) string {
	names := append([]string(nil), authn.GetAttributeNames()...)
	slices.Sort(names)
	var b strings.Builder
	for _, name := range names {
		writeLenPrefixed(&b, name)
		writeLenPrefixed(&b, encodeAuthAttribute(authn.GetAttribute(name)))
	}
	return fingerprint(b.String())
}

func encodeAuthAttribute(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

// authInputFingerprint hashes the headers and query parameters a confighttp
// authenticator receives. Query parameters the authenticator is not
// configured to see are left out, matching authInterceptor.
func authInputFingerprint(r *http.Request, params []string) string {
	names := make([]string, 0, len(r.Header))
	for name := range r.Header {
		if _, skip := volatileSessionHeaders[name]; skip || name == sessionBindingHeader {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, name := range names {
		writeLenPrefixed(&b, name)
		for _, value := range r.Header.Values(name) {
			writeLenPrefixed(&b, value)
		}
	}
	// Separate headers from query parameters so a header cannot collide with one.
	b.WriteByte(0)
	paramNames := append([]string(nil), params...)
	slices.Sort(paramNames)
	query := r.URL.Query()
	for _, name := range paramNames {
		writeLenPrefixed(&b, name)
		for _, value := range query[name] {
			writeLenPrefixed(&b, value)
		}
	}
	return fingerprint(b.String())
}

func writeLenPrefixed(b *strings.Builder, s string) {
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
	b.WriteByte(0)
}

func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// checkSessionCaller rejects a request whose recorded caller differs from the
// request that opened its session, and gives the request its own client
// metadata and, with propagateBearerToken, its own bearer token in place of the
// opener's. The recorded caller is the tenant, the forwarded headers, and,
// when the request was authenticated, a fingerprint of the principal or of
// the authenticator's inputs.
func checkSessionCaller(logger *zap.Logger, propagateBearerToken bool) mcp.Middleware {
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
					if name == sessionBindingHeader {
						name = authenticatedIdentityMismatch
					}
					return reject(name)
				}
			}
			if propagateBearerToken {
				token, _ := bearertoken.TokenFromHTTPHeader(header)
				if _, ok := bearertoken.GetBearerToken(ctx); ok && token == "" {
					// The opener's token cannot be taken out of the context, so a
					// request that carries none is refused rather than served with it.
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
		// The binding is an internal fingerprint, not a client header.
		if name == sessionBindingHeader {
			continue
		}
		md[name] = slices.Clone(values)
	}
	info.Metadata = client.NewMetadata(md)
	return client.NewContext(ctx, info)
}
