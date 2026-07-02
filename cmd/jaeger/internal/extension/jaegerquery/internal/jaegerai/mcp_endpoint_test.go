// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newTestMCPMux mounts an mcpSessionHandler on a mux (both the slash and
// no-slash session patterns, as RegisterRoutes does), backed by a stub
// telemetry handler that records the (post-strip) path it saw.
func newTestMCPMux(t *testing.T, basePath string, streams *sessionStreams) (*http.ServeMux, *string) {
	t.Helper()
	var seenPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("telemetry"))
	})
	h := &mcpSessionHandler{telemetryHandler: stub, streams: streams, basePath: basePath, logger: zap.NewNop()}
	mux := http.NewServeMux()
	mux.Handle(basePath+routeMCPSession, h)
	mux.Handle(basePath+routeMCPSessionNoSlash, h)
	return mux, &seenPath
}

func TestMCPSessionHandlerServesActiveSession(t *testing.T) {
	// The session prefix must be stripped so the telemetry handler sees its own
	// root, for the trailing-slash, subpath, and no-slash forms alike.
	cases := []struct {
		reqSuffix    string
		wantSeenPath string
	}{
		{"/api/ai/mcp/sess-1/mcp", "/mcp"},
		{"/api/ai/mcp/sess-1/", "/"},
		{"/api/ai/mcp/sess-1", "/"}, // no trailing slash normalizes to root
	}
	for _, basePath := range []string{"", "/jaeger"} {
		for _, tc := range cases {
			t.Run(basePath+tc.reqSuffix, func(t *testing.T) {
				streams := newSessionStreams()
				streams.set("sess-1", testStreamingClient())
				mux, seenPath := newTestMCPMux(t, basePath, streams)

				rr := httptest.NewRecorder()
				mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, basePath+tc.reqSuffix, http.NoBody))

				require.Equal(t, http.StatusOK, rr.Code)
				assert.Equal(t, "telemetry", rr.Body.String())
				assert.Equal(t, tc.wantSeenPath, *seenPath)
			})
		}
	}
}

func TestMCPSessionHandlerRejectsUnknownSession(t *testing.T) {
	streams := newSessionStreams() // empty: no active sessions
	mux, seenPath := newTestMCPMux(t, "", streams)

	for _, reqPath := range []string{"/api/ai/mcp/ghost/mcp", "/api/ai/mcp/ghost"} {
		t.Run(reqPath, func(t *testing.T) {
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, reqPath, http.NoBody))
			require.Equal(t, http.StatusNotFound, rr.Code)
			assert.Empty(t, *seenPath, "telemetry handler must not be reached for an unknown session")
		})
	}
}
