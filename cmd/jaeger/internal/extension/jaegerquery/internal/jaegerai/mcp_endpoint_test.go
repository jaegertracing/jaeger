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

// newTestMCPMux mounts an mcpSessionHandler on a mux at the given base path,
// backed by a stub telemetry handler that records the (post-strip) path it saw.
func newTestMCPMux(t *testing.T, basePath string, streams *sessionStreams) (*http.ServeMux, *string) {
	t.Helper()
	var seenPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("telemetry"))
	})
	h := &mcpSessionHandler{telemetry: stub, streams: streams, basePath: basePath, logger: zap.NewNop()}
	mux := http.NewServeMux()
	mux.Handle(basePath+routeMCPSession, h)
	return mux, &seenPath
}

func TestMCPSessionHandlerServesActiveSession(t *testing.T) {
	for _, basePath := range []string{"", "/jaeger"} {
		t.Run("base path "+basePath, func(t *testing.T) {
			streams := newSessionStreams()
			streams.set("sess-1", testStreamingClient())
			mux, seenPath := newTestMCPMux(t, basePath, streams)

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, basePath+"/api/ai/mcp/sess-1/mcp", http.NoBody))

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "telemetry", rr.Body.String())
			// Session prefix stripped: the telemetry handler sees a clean root.
			assert.Equal(t, "/mcp", *seenPath)
		})
	}
}

func TestMCPSessionHandlerRejectsUnknownSession(t *testing.T) {
	streams := newSessionStreams() // empty: no active sessions
	mux, seenPath := newTestMCPMux(t, "", streams)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/ghost/mcp", http.NoBody))

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Empty(t, *seenPath, "telemetry handler must not be reached for an unknown session")
}
