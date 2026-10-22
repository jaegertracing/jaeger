// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depsmocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestRegister(t *testing.T) {
	telset := telemetry.NoopSettings()
	telset.Logger = zaptest.NewLogger(t)
	traceReader := &tracestoremocks.Reader{}
	querySvc := querysvc.NewQueryService(traceReader, &depsmocks.Reader{}, querysvc.QueryServiceOptions{})

	register := func(t *testing.T, cfg Config) (*http.ServeMux, io.Closer) {
		r := http.NewServeMux()
		closer, err := Register(context.Background(), r, RegisterParams{
			Config:       &cfg,
			QueryService: querySvc,
			Tenancy:      tenancy.NewManager(&tenancy.Options{}),
			Telemetry:    telset,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closer.Close()) })
		return r, closer
	}
	status := func(r *http.ServeMux, method, path string) int {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(`{}`)))
		return rr.Code
	}

	t.Run("invalid config mounts nothing", func(t *testing.T) {
		r, _ := register(t, Config{})
		require.Equal(t, http.StatusNotFound, status(r, http.MethodPost, "/api/ai/chat"))
		require.Equal(t, http.StatusNotFound, status(r, http.MethodGet, "/api/ai/mcp/"))
	})

	t.Run("mcp without agent_url mounts the shared endpoint only", func(t *testing.T) {
		cfg := validConfig()
		cfg.AgentURL = ""
		cfg.MCP = configoptional.Some(MCPConfig{})
		r, _ := register(t, cfg)
		// The MCP transport judges the probe itself; anything but 404 proves the mount.
		require.NotEqual(t, http.StatusNotFound, status(r, http.MethodGet, "/api/ai/mcp/"))
		require.Equal(t, http.StatusNotFound, status(r, http.MethodPost, "/api/ai/chat"))
	})

	t.Run("agent_url and mcp mount both surfaces", func(t *testing.T) {
		cfg := validConfig()
		cfg.MCP = configoptional.Some(MCPConfig{})
		r, _ := register(t, cfg)
		require.NotEqual(t, http.StatusNotFound, status(r, http.MethodGet, "/api/ai/mcp/"))
		require.Equal(t, http.StatusBadRequest, status(r, http.MethodPost, "/api/ai/chat"))
	})

	t.Run("base path with a trailing slash yields no double slash in the mount", func(t *testing.T) {
		cfg := validConfig()
		cfg.AgentURL = ""
		cfg.MCP = configoptional.Some(MCPConfig{})
		r := http.NewServeMux()
		closer, err := Register(context.Background(), r, RegisterParams{
			Config:       &cfg,
			BasePath:     "/jaeger/",
			QueryService: querySvc,
			Tenancy:      tenancy.NewManager(&tenancy.Options{}),
			Telemetry:    telset,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closer.Close()) })
		require.NotEqual(t, http.StatusNotFound, status(r, http.MethodGet, "/jaeger/api/ai/mcp/"))
	})

	t.Run("unreadable skills dir fails registration", func(t *testing.T) {
		cfg := validConfig()
		cfg.MCP = configoptional.Some(MCPConfig{SkillsDir: filepath.Join(t.TempDir(), "missing")})
		closer, err := Register(context.Background(), http.NewServeMux(), RegisterParams{Config: &cfg, Telemetry: telset})
		require.Error(t, err)
		require.Nil(t, closer)
	})
}

func TestClosersJoinErrors(t *testing.T) {
	cs := closers{
		failingCloser("first"),
		failingCloser("second"),
	}
	err := cs.Close()
	require.ErrorContains(t, err, "first")
	require.ErrorContains(t, err, "second")
}

type failingCloser string

func (f failingCloser) Close() error { return errors.New(string(f)) }
