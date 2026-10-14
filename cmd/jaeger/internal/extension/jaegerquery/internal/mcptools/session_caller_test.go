// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// testCaller is the HTTP transport of a test MCP client: it sends its headers
// on every request, so changing them mid-session makes another caller use the
// same MCP session. rawQuery, when hasQuery is set, does the same for the
// authenticator's query parameters.
type testCaller struct {
	mu       sync.Mutex
	header   http.Header
	rawQuery string
	hasQuery bool
}

func (c *testCaller) update(change func(http.Header)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	change(c.header)
}

func (c *testCaller) setQuery(q string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rawQuery = q
	c.hasQuery = true
}

func (c *testCaller) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	r = r.Clone(r.Context())
	for name, values := range c.header {
		r.Header[name] = slices.Clone(values)
	}
	if c.hasQuery {
		r.URL.RawQuery = c.rawQuery
	}
	c.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

func connectTestClientAs(t *testing.T, url string, caller *testCaller) *mcp.ClientSession {
	t.Helper()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := mcpClient.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: caller},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })
	return session
}

func serveTestHandler(t *testing.T, handler http.Handler) string {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts.URL
}

// storageCalls records, for each GetServices call that reaches storage, what
// the given function reads from its context.
type storageCalls struct {
	mu   sync.Mutex
	seen []string
}

func (s *storageCalls) queryService(read func(context.Context) string) *querysvc.QueryService {
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Run(func(args mock.Arguments) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.seen = append(s.seen, read(args.Get(0).(context.Context)))
	}).Return([]string{"svc"}, nil)
	return querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
}

func (s *storageCalls) get() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

func bearerToken(ctx context.Context) string {
	token, _ := bearertoken.GetBearerToken(ctx)
	return token
}

func propagateBearerToken(h http.Handler) http.Handler {
	return bearertoken.PropagationHandler(zap.NewNop(), h)
}

// testAuth stands in for the client.AuthData a confighttp authenticator sets.
type testAuth string

func (a testAuth) GetAttribute(string) any   { return string(a) }
func (testAuth) GetAttributeNames() []string { return []string{"subject"} }

// authenticate stands in for a confighttp authenticator that takes the caller
// from the Authorization header.
func authenticate(next http.Handler) http.Handler {
	return authenticateFrom(func(r *http.Request) client.AuthData {
		return testAuth(r.Header.Get("Authorization"))
	})(next)
}

// authenticateFrom stands in for a confighttp authenticator. sources is every
// header plus the configured query parameters; the stand-in reads whichever of
// those the test's authenticator uses, and may return a principal or only a
// context value.
func authenticateFrom(principal func(*http.Request) client.AuthData) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if principal != nil {
				info := client.FromContext(ctx)
				info.Auth = principal(r)
				ctx = client.NewContext(ctx, info)
			}
			if id := requestIdentity(r); id != "" || principal == nil {
				ctx = context.WithValue(ctx, ctxIdentityKey{}, requestIdentity(r))
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ctxIdentityKey is the custom context value an authenticator may use instead
// of client.Info.Auth. The extensionauth contract leaves the key undefined.
type ctxIdentityKey struct{}

func requestIdentity(r *http.Request) string {
	if id := r.Header.Get("X-Api-Key"); id != "" {
		return id
	}
	if c, err := r.Cookie("session"); err == nil {
		return c.Value
	}
	return r.URL.Query().Get("token")
}

func contextIdentity(ctx context.Context) string {
	id, _ := ctx.Value(ctxIdentityKey{}).(string)
	return id
}

// emptyAuth is a principal with no attributes. The authenticator accepted the
// request, but the caller lives in a context value rather than Auth.
type emptyAuth struct{}

func (emptyAuth) GetAttribute(string) any     { return nil }
func (emptyAuth) GetAttributeNames() []string { return nil }

func authenticatedSubject(ctx context.Context) string {
	if auth := client.FromContext(ctx).Auth; auth != nil {
		return auth.GetAttribute("subject").(string)
	}
	return ""
}

func authenticatedConfig(params ...string) func(*Config) {
	return func(cfg *Config) {
		cfg.Authenticated = true
		cfg.AuthRequestParameters = params
	}
}

func principalFromCookie(r *http.Request) client.AuthData {
	c, err := r.Cookie("session")
	if err != nil {
		return testAuth("")
	}
	return testAuth(c.Value)
}

func principalFromHeader(name string) func(*http.Request) client.AuthData {
	return func(r *http.Request) client.AuthData {
		return testAuth(r.Header.Get(name))
	}
}

func principalFromQuery(r *http.Request) client.AuthData {
	return testAuth(r.URL.Query().Get("token"))
}

func TestSessionRejectsRequestsFromAnotherCaller(t *testing.T) {
	forwarded := []headerforwarding.ForwardedHeader{{HTTPName: "X-User"}}
	forwardHeaders := func(cfg *Config) { cfg.HeaderForwarding = forwarded }
	captureForwarded := func(h http.Handler) http.Handler {
		return headerforwarding.HTTPServerMiddleware(forwarded, h)
	}
	forwardedValues := func(ctx context.Context) string {
		var values []string
		for _, captured := range headerforwarding.CapturedFromContext(ctx) {
			values = append(values, captured.Value)
		}
		return strings.Join(values, ",")
	}
	tests := []struct {
		name         string
		tenancy      bool
		configure    func(*Config)
		wrap         func(http.Handler) http.Handler
		read         func(context.Context) string
		opener       http.Header
		change       func(http.Header)
		wantSeen     string
		wantMismatch string
	}{
		{
			name:         "different tenant",
			tenancy:      true,
			read:         tenancy.GetTenant,
			opener:       http.Header{"X-Tenant": {"tenant-a"}},
			change:       func(h http.Header) { h.Set("X-Tenant", "tenant-b") },
			wantSeen:     "tenant-a",
			wantMismatch: "X-Tenant",
		},
		{
			name:         "different forwarded header",
			configure:    forwardHeaders,
			wrap:         captureForwarded,
			read:         forwardedValues,
			opener:       http.Header{"X-User": {"alice"}},
			change:       func(h http.Header) { h.Set("X-User", "mallory") },
			wantSeen:     "alice",
			wantMismatch: "X-User",
		},
		{
			name:         "forwarded header added",
			configure:    forwardHeaders,
			wrap:         captureForwarded,
			read:         forwardedValues,
			opener:       http.Header{},
			change:       func(h http.Header) { h.Set("X-User", "bob") },
			wantSeen:     "",
			wantMismatch: "X-User",
		},
		{
			name:         "different credentials of an authenticated caller",
			wrap:         authenticate,
			read:         authenticatedSubject,
			opener:       http.Header{"Authorization": {"Bearer alice"}},
			change:       func(h http.Header) { h.Set("Authorization", "Bearer mallory") },
			wantSeen:     "Bearer alice",
			wantMismatch: authenticatedIdentityMismatch,
		},
		{
			name:   "different cookie principal",
			wrap:   authenticateFrom(principalFromCookie),
			read:   authenticatedSubject,
			opener: http.Header{"Cookie": {"session=alice"}},
			change: func(h http.Header) { h.Set("Cookie", "session=mallory") },
			// The opener's context is what storage sees. A second cookie must
			// not be served as the opener.
			wantSeen:     "alice",
			wantMismatch: authenticatedIdentityMismatch,
		},
		{
			name:         "different custom-header principal",
			wrap:         authenticateFrom(principalFromHeader("X-Api-Key")),
			read:         authenticatedSubject,
			opener:       http.Header{"X-Api-Key": {"alice"}},
			change:       func(h http.Header) { h.Set("X-Api-Key", "mallory") },
			wantSeen:     "alice",
			wantMismatch: authenticatedIdentityMismatch,
		},
		{
			name:         "custom identity context and a different credential header",
			configure:    authenticatedConfig(),
			wrap:         authenticateFrom(nil),
			read:         contextIdentity,
			opener:       http.Header{"X-Api-Key": {"alice"}},
			change:       func(h http.Header) { h.Set("X-Api-Key", "mallory") },
			wantSeen:     "alice",
			wantMismatch: authenticatedIdentityMismatch,
		},
		{
			name:         "empty principal falls back to authenticator inputs",
			wrap:         authenticateFrom(func(*http.Request) client.AuthData { return emptyAuth{} }),
			read:         contextIdentity,
			opener:       http.Header{"Cookie": {"session=alice"}},
			change:       func(h http.Header) { h.Set("Cookie", "session=mallory") },
			wantSeen:     "alice",
			wantMismatch: authenticatedIdentityMismatch,
		},
		{
			name:         "custom identity context gains a credential header",
			configure:    authenticatedConfig(),
			wrap:         authenticateFrom(nil),
			read:         contextIdentity,
			opener:       http.Header{},
			change:       func(h http.Header) { h.Set("X-Api-Key", "bob") },
			wantSeen:     "",
			wantMismatch: authenticatedIdentityMismatch,
		},
		{
			name:         "propagated bearer token dropped",
			configure:    func(cfg *Config) { cfg.BearerTokenPropagation = true },
			wrap:         propagateBearerToken,
			read:         bearerToken,
			opener:       http.Header{"Authorization": {"Bearer alice"}},
			change:       func(h http.Header) { h.Del("Authorization") },
			wantSeen:     "alice",
			wantMismatch: "bearer token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			telset := telemetry.NoopSettings()
			telset.Logger = zap.New(core)
			var calls storageCalls
			cfg := DefaultConfig()
			if tt.configure != nil {
				tt.configure(&cfg)
			}
			h := NewHandler(telset, calls.queryService(tt.read), tenancy.NewManager(&tenancy.Options{Enabled: tt.tenancy}), cfg)
			t.Cleanup(func() { require.NoError(t, h.Close()) })
			var handler http.Handler = h
			if tt.wrap != nil {
				handler = tt.wrap(handler)
			}
			caller := &testCaller{header: tt.opener}
			session := connectTestClientAs(t, serveTestHandler(t, handler), caller)

			_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
			require.NoError(t, err, "a later request from the caller that opened the session is served")

			caller.update(tt.change)
			_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
			require.ErrorContains(t, err, errSessionCallerMismatch.Message)
			assert.Equal(t, []string{tt.wantSeen}, calls.get(), "only the opener's request reaches storage")

			entries := logs.AllUntimed()
			require.Len(t, entries, 1, "the rejection is logged")
			assert.Equal(t, zap.InfoLevel, entries[0].Level)
			assert.Equal(t, map[string]any{"method": mcpMethodToolsCall, "mismatch": tt.wantMismatch},
				entries[0].ContextMap(), "the log names the header, not its value")
		})
	}
}

// TestSessionRejectsDifferentQueryPrincipal covers an authenticator that reads
// a query parameter (confighttp auth.request_params) and returns that caller
// as client.Info.Auth. The two requests carry the same headers.
func TestSessionRejectsDifferentQueryPrincipal(t *testing.T) {
	var calls storageCalls
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(authenticatedSubject), tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{hasQuery: true, rawQuery: "token=alice"}
	session := connectTestClientAs(t, serveTestHandler(t, authenticateFrom(principalFromQuery)(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.setQuery("token=mallory")
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.ErrorContains(t, err, errSessionCallerMismatch.Message)
	assert.Equal(t, []string{"alice"}, calls.get())
}

// TestSessionRejectsQueryParamOnCustomIdentity covers an authenticator that
// never sets client.Info.Auth and instead stores the caller, taken from
// auth.request_params, in its own context value.
func TestSessionRejectsQueryParamOnCustomIdentity(t *testing.T) {
	var calls storageCalls
	cfg := DefaultConfig()
	authenticatedConfig("token")(&cfg)
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(contextIdentity), tenancy.NewManager(&tenancy.Options{}), cfg)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{hasQuery: true, rawQuery: "token=alice"}
	session := connectTestClientAs(t, serveTestHandler(t, authenticateFrom(nil)(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.setQuery("token=mallory")
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.ErrorContains(t, err, errSessionCallerMismatch.Message)
	assert.Equal(t, []string{"alice"}, calls.get())
}

// TestSessionAllowsSamePrincipalWhenBearerHeaderChanges covers a session bound
// to a cookie principal: refreshing Authorization, which this authenticator
// does not read, does not make the request someone else.
func TestSessionAllowsSamePrincipalWhenBearerHeaderChanges(t *testing.T) {
	var calls storageCalls
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(authenticatedSubject), tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{header: http.Header{"Cookie": {"session=alice"}}}
	session := connectTestClientAs(t, serveTestHandler(t, authenticateFrom(principalFromCookie)(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.update(func(header http.Header) {
		header.Set("Authorization", "Bearer refreshed")
		header.Set("Traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	})
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "alice"}, calls.get())
}

// TestSessionAllowsVolatileHeadersForSameCustomIdentity covers the headers a
// client or a proxy changes between the request that opens a session and a
// later call. They are not authenticator inputs, so the same caller proceeds.
func TestSessionAllowsVolatileHeadersForSameCustomIdentity(t *testing.T) {
	var calls storageCalls
	cfg := DefaultConfig()
	authenticatedConfig()(&cfg)
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(contextIdentity), tenancy.NewManager(&tenancy.Options{}), cfg)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{header: http.Header{"X-Api-Key": {"alice"}}}
	session := connectTestClientAs(t, serveTestHandler(t, authenticateFrom(nil)(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.update(func(header http.Header) {
		header.Set("Traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
		header.Set("X-Request-Id", "req-2")
		header.Set("Mcp-Protocol-Version", "2025-11-25")
	})
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "alice"}, calls.get())
}

// TestSessionAllowsTokenRefreshNothingReads covers a client that refreshes its
// token mid-session when neither an authenticator nor token propagation reads
// it: the request does not change who the session serves, so it is not refused.
func TestSessionAllowsTokenRefreshNothingReads(t *testing.T) {
	for _, header := range []string{"Authorization", "X-Forwarded-Access-Token"} {
		t.Run(header, func(t *testing.T) {
			var calls storageCalls
			h := NewHandler(telemetry.NoopSettings(), calls.queryService(bearerToken), tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
			t.Cleanup(func() { require.NoError(t, h.Close()) })
			caller := &testCaller{header: http.Header{header: {"Bearer token-1"}}}
			session := connectTestClientAs(t, serveTestHandler(t, h), caller)

			_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
			require.NoError(t, err)
			caller.update(func(hdr http.Header) { hdr.Set(header, "Bearer token-2") })
			_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
			require.NoError(t, err)
			assert.Equal(t, []string{"", ""}, calls.get())
		})
	}
}

// TestSessionPropagatesCurrentBearerToken covers bearer_token_propagation:
// storage checks the token on every query, so a request on a session must
// reach it with its own token, not the opener's.
func TestSessionPropagatesCurrentBearerToken(t *testing.T) {
	var calls storageCalls
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(bearerToken), tenancy.NewManager(&tenancy.Options{}), propagationConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{header: http.Header{"Authorization": {"Bearer alice"}}}
	session := connectTestClientAs(t, serveTestHandler(t, propagateBearerToken(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.update(func(hdr http.Header) { hdr.Set("Authorization", "Bearer bob") })
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob"}, calls.get())
}

// TestSessionPropagatesBearerTokenAddedAfterOpen covers a session opened
// without a token under bearer_token_propagation: a later request that carries
// one reaches storage with it, as it would on a session of its own.
func TestSessionPropagatesBearerTokenAddedAfterOpen(t *testing.T) {
	var calls storageCalls
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(bearerToken), tenancy.NewManager(&tenancy.Options{}), propagationConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{header: http.Header{}}
	session := connectTestClientAs(t, serveTestHandler(t, propagateBearerToken(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.update(func(hdr http.Header) { hdr.Set("Authorization", "Bearer bob") })
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.Equal(t, []string{"", "bob"}, calls.get())
}

func propagationConfig() Config {
	cfg := DefaultConfig()
	cfg.BearerTokenPropagation = true
	return cfg
}

// TestSessionServesCurrentClientMetadata covers query interceptors that read
// the caller from client metadata (http.include_metadata): a request on a
// session must carry its own headers there, not those of the request that
// opened the session.
func TestSessionServesCurrentClientMetadata(t *testing.T) {
	includeMetadata := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			md := r.Header.Clone()
			md.Set(client.MetadataHostName, r.Host)
			ctx := client.NewContext(r.Context(), client.Info{Metadata: client.NewMetadata(md)})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	var calls storageCalls
	svc := calls.queryService(func(ctx context.Context) string {
		md := client.FromContext(ctx).Metadata
		return strings.Join(md.Get("X-Caller"), ",") + "@" + strings.Join(md.Get(client.MetadataHostName), ",")
	})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	caller := &testCaller{header: http.Header{"X-Caller": {"alice"}}}
	session := connectTestClientAs(t, serveTestHandler(t, includeMetadata(h)), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	caller.update(func(header http.Header) { header.Set("X-Caller", "bob") })
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)

	seen := calls.get()
	require.Len(t, seen, 2)
	alice, aliceHost, _ := strings.Cut(seen[0], "@")
	bob, bobHost, _ := strings.Cut(seen[1], "@")
	assert.Equal(t, "alice", alice)
	assert.Equal(t, "bob", bob)
	assert.NotEmpty(t, aliceHost)
	assert.Equal(t, aliceHost, bobHost, "the opener's Host is kept")
}

// TestHandlerAddReceivingMiddlewareRunsAfterCallerCheck pins the caller check
// ahead of middleware layered on later, such as the AI gateway's, which answers
// its UI tool calls itself without reaching the telemetry tools.
func TestHandlerAddReceivingMiddlewareRunsAfterCallerCheck(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{Enabled: true}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })

	var answered atomic.Int32
	h.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != mcpMethodToolsCall {
				return next(ctx, method, req)
			}
			answered.Add(1)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "answered"}}}, nil
		}
	})

	caller := &testCaller{header: http.Header{"X-Tenant": {"tenant-a"}}}
	session := connectTestClientAs(t, serveTestHandler(t, h), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "ui_tool"})
	require.NoError(t, err)

	caller.update(func(header http.Header) { header.Set("X-Tenant", "tenant-b") })
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "ui_tool"})
	require.ErrorContains(t, err, errSessionCallerMismatch.Message)
	assert.Equal(t, int32(1), answered.Load(), "the middleware answered only the opener's call")
}

// TestSessionDeleteFromAnotherCallerEndsSession pins what the caller check does
// not cover: the SDK serves DELETE before any receiving middleware runs, so a
// caller holding another tenant's session ID can end that session, though it
// is served nothing from it.
func TestSessionDeleteFromAnotherCallerEndsSession(t *testing.T) {
	var calls storageCalls
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(tenancy.GetTenant), tenancy.NewManager(&tenancy.Options{Enabled: true}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	url := serveTestHandler(t, h)
	session := connectTestClientAs(t, url, &testCaller{header: http.Header{"X-Tenant": {"tenant-a"}}})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, url, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Mcp-Session-Id", session.ID())
	req.Header.Set("X-Tenant", "tenant-b")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.ErrorIs(t, err, mcp.ErrSessionMissing)
	assert.Empty(t, calls.get())
}

// TestCheckSessionCallerSkipsSessionsNotOpenedOverHTTP covers a session with no
// HTTP request behind it, which has no caller headers to compare.
func TestCheckSessionCallerSkipsSessionsNotOpenedOverHTTP(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := h.server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })

	_, err = session.ListTools(t.Context(), &mcp.ListToolsParams{})
	require.NoError(t, err)
}

// TestSessionBindingHeaderStaysOutOfClientMetadata covers include_metadata:
// the internal fingerprint must not be copied into the client metadata a
// query interceptor reads.
func TestSessionBindingHeaderStaysOutOfClientMetadata(t *testing.T) {
	includeMetadata := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			md := r.Header.Clone()
			md.Set(client.MetadataHostName, r.Host)
			ctx := client.NewContext(r.Context(), client.Info{Metadata: client.NewMetadata(md)})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	var calls storageCalls
	svc := calls.queryService(func(ctx context.Context) string {
		md := client.FromContext(ctx).Metadata
		if len(md.Get(sessionBindingHeader)) > 0 {
			return "leaked"
		}
		return strings.Join(md.Get("X-Api-Key"), ",")
	})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	handler := includeMetadata(authenticateFrom(principalFromHeader("X-Api-Key"))(h))
	caller := &testCaller{header: http.Header{"X-Api-Key": {"alice"}}}
	session := connectTestClientAs(t, serveTestHandler(t, handler), caller)

	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice"}, calls.get())
}

func TestSessionBindingSkipsUnauthenticatedRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer alice")
	assert.Empty(t, sessionBinding(req, DefaultConfig()))
}

func TestRecordSessionCallerNilHeaderStillBinds(t *testing.T) {
	var got string
	h := recordSessionCaller(tenancy.NewManager(&tenancy.Options{}), Config{Authenticated: true}, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(sessionBindingHeader)
		_, ok := r.Context().Value(sessionCallerKey{}).(http.Header)
		assert.True(t, ok)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", http.NoBody)
	req.Header = nil
	h.ServeHTTP(httptest.NewRecorder(), req)
	assert.NotEmpty(t, got)
}

type mapAuth map[string]any

func (m mapAuth) GetAttribute(name string) any { return m[name] }
func (m mapAuth) GetAttributeNames() []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}

type failingAttr struct{}

func (failingAttr) MarshalJSON() ([]byte, error) {
	return nil, http.ErrNotSupported
}

func TestPrincipalFingerprint(t *testing.T) {
	left := mapAuth{"b": "2", "a": []string{"x", "y"}}
	right := mapAuth{"a": []string{"x", "y"}, "b": "2"}
	assert.Equal(t, principalFingerprint(left), principalFingerprint(right))

	changed := mapAuth{"a": []string{"x", "y"}, "b": "3"}
	assert.NotEqual(t, principalFingerprint(left), principalFingerprint(changed))

	// json.Marshal cannot encode failingAttr, so the fingerprint falls back to
	// fmt's default form and stays stable for the same value.
	first := mapAuth{"v": failingAttr{}}
	second := mapAuth{"v": failingAttr{}}
	assert.Equal(t, "{}", encodeAuthAttribute(failingAttr{}))
	assert.Equal(t, principalFingerprint(first), principalFingerprint(second))
	assert.NotEqual(t, principalFingerprint(left), principalFingerprint(first))
}

func TestAuthInputFingerprint(t *testing.T) {
	opened := httptest.NewRequest(http.MethodPost, "http://example.com/mcp?token=alice&other=1", http.NoBody)
	opened.Header.Set("X-Api-Key", "alice")
	opened.Header.Set(sessionBindingHeader, "client-supplied")

	later := opened.Clone(opened.Context())
	later.Header = opened.Header.Clone()
	later.Header.Set("Mcp-Session-Id", "sess-1")
	later.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	later.Header.Set("Content-Length", "99")
	later.Header.Set("Traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	later.Header.Set("X-Request-Id", "req-2")
	assert.Equal(t, authInputFingerprint(opened, []string{"token"}), authInputFingerprint(later, []string{"token"}),
		"protocol, framing, and tracing headers are not authenticator inputs")

	// A query parameter the authenticator is not configured to receive.
	other := opened.Clone(opened.Context())
	other.Header = opened.Header.Clone()
	other.URL.RawQuery = "token=alice&other=2"
	assert.Equal(t, authInputFingerprint(opened, []string{"token"}), authInputFingerprint(other, []string{"token"}))

	changedKey := opened.Clone(opened.Context())
	changedKey.Header = opened.Header.Clone()
	changedKey.Header.Set("X-Api-Key", "mallory")
	assert.NotEqual(t, authInputFingerprint(opened, []string{"token"}), authInputFingerprint(changedKey, []string{"token"}))

	added := opened.Clone(opened.Context())
	added.Header = opened.Header.Clone()
	added.Header.Set("X-Extra", "1")
	assert.NotEqual(t, authInputFingerprint(opened, []string{"token"}), authInputFingerprint(added, []string{"token"}))

	changedToken := opened.Clone(opened.Context())
	changedToken.Header = opened.Header.Clone()
	changedToken.URL.RawQuery = "token=mallory&other=1"
	assert.NotEqual(t, authInputFingerprint(opened, []string{"token"}), authInputFingerprint(changedToken, []string{"token"}))

	absentToken := opened.Clone(opened.Context())
	absentToken.Header = opened.Header.Clone()
	absentToken.URL.RawQuery = "other=1"
	assert.NotEqual(t, authInputFingerprint(opened, []string{"token"}), authInputFingerprint(absentToken, []string{"token"}))
}
