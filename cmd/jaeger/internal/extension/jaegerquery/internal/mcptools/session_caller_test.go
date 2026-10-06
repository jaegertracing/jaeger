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
// same MCP session.
type testCaller struct {
	mu     sync.Mutex
	header http.Header
}

func (c *testCaller) update(change func(http.Header)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	change(c.header)
}

func (c *testCaller) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	r = r.Clone(r.Context())
	for name, values := range c.header {
		r.Header[name] = slices.Clone(values)
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := client.FromContext(r.Context())
		info.Auth = testAuth(r.Header.Get("Authorization"))
		next.ServeHTTP(w, r.WithContext(client.NewContext(r.Context(), info)))
	})
}

func authenticatedSubject(ctx context.Context) string {
	if auth := client.FromContext(ctx).Auth; auth != nil {
		return auth.GetAttribute("subject").(string)
	}
	return ""
}

func TestSessionRejectsRequestsFromAnotherCaller(t *testing.T) {
	tests := []struct {
		name         string
		tenancy      bool
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
			name: "different forwarded header",
			wrap: func(h http.Handler) http.Handler {
				return headerforwarding.HTTPServerMiddleware([]headerforwarding.ForwardedHeader{{HTTPName: "X-User"}}, h)
			},
			read: func(ctx context.Context) string {
				var values []string
				for _, captured := range headerforwarding.CapturedFromContext(ctx) {
					values = append(values, captured.Value)
				}
				return strings.Join(values, ",")
			},
			opener:       http.Header{"X-User": {"alice"}},
			change:       func(h http.Header) { h.Set("X-User", "mallory") },
			wantSeen:     "alice",
			wantMismatch: "X-User",
		},
		{
			name:         "different credentials of an authenticated caller",
			wrap:         authenticate,
			read:         authenticatedSubject,
			opener:       http.Header{"Authorization": {"Bearer alice"}},
			change:       func(h http.Header) { h.Set("Authorization", "Bearer mallory") },
			wantSeen:     "Bearer alice",
			wantMismatch: "Authorization",
		},
		{
			name:         "propagated bearer token dropped",
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
			h := NewHandler(telset, calls.queryService(tt.read), tenancy.NewManager(&tenancy.Options{Enabled: tt.tenancy}), DefaultConfig())
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
	h := NewHandler(telemetry.NoopSettings(), calls.queryService(bearerToken), tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
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
