// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/anonymizer/app"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

type queryServer struct {
	api_v3.UnimplementedQueryServiceServer
	traces  ptrace.Traces
	traceID string
}

func (s *queryServer) GetTrace(r *api_v3.GetTraceRequest, stream api_v3.QueryService_GetTraceServer) error {
	s.traceID = r.GetTraceId()
	if s.traces.SpanCount() == 0 {
		return status.Error(codes.NotFound, "trace not found")
	}
	data := jptrace.TracesData(s.traces)
	return stream.Send(&data)
}

func startQueryServer(t *testing.T, handler *queryServer) string {
	server := grpc.NewServer()
	api_v3.RegisterQueryServiceServer(server, handler)
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	var exited sync.WaitGroup
	exited.Go(func() { _ = server.Serve(lis) })
	t.Cleanup(func() {
		server.Stop()
		exited.Wait()
	})
	return lis.Addr().String()
}

func readTraces(t *testing.T, path string) ptrace.Traces {
	dat, err := os.ReadFile(path)
	require.NoError(t, err)
	var unmarshaler ptrace.JSONUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(dat)
	require.NoError(t, err)
	return traces
}

func TestRunWritesOTLPFiles(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "api")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("delete")
	span.Attributes().PutStr("user.email", "a@example.com")

	handler := &queryServer{traces: traces}
	outputDir := t.TempDir()
	options := &app.Options{
		QueryGRPCHostPort: startQueryServer(t, handler),
		OutputDir:         outputDir,
		TraceID:           "abc", // short IDs are padded once, and the file names keep the spelling given
	}
	require.NoError(t, run(options, zap.NewNop()))

	assert.Equal(t, "00000000000000000000000000000abc", handler.traceID)

	prefix := filepath.Join(outputDir, "abc")
	original := readTraces(t, prefix+".original.json")
	assert.Equal(t, traces, original)

	anonymized := readTraces(t, prefix+".anonymized.json")
	got := anonymized.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.NotEqual(t, "delete", got.Name())
	assert.Equal(t, 0, got.Attributes().Len(), "custom attributes are dropped by default")

	mapping, err := os.ReadFile(prefix + ".mapping.json")
	require.NoError(t, err)
	assert.Contains(t, string(mapping), `"api"`)

	_, err = os.Stat(prefix + ".anonymized-ui-trace.json")
	assert.ErrorIs(t, err, os.ErrNotExist, "the UI-format file is no longer written")
}

func TestRunErrors(t *testing.T) {
	t.Run("invalid trace ID", func(t *testing.T) {
		err := run(&app.Options{TraceID: "not-hex", OutputDir: t.TempDir()}, zap.NewNop())
		require.ErrorContains(t, err, `invalid trace ID "not-hex"`)
	})

	t.Run("trace not found", func(t *testing.T) {
		options := &app.Options{
			QueryGRPCHostPort: startQueryServer(t, &queryServer{traces: ptrace.NewTraces()}),
			OutputDir:         t.TempDir(),
			TraceID:           "abc",
		}
		err := run(options, zap.NewNop())
		require.ErrorContains(t, err, "error while querying for trace")
	})

	t.Run("unreadable earlier mapping", func(t *testing.T) {
		outputDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(outputDir, "abc.mapping.json"), 0o700))
		err := run(&app.Options{TraceID: "abc", OutputDir: outputDir}, zap.NewNop())
		require.ErrorContains(t, err, "cannot load previous mapping")
	})
}

func TestInitTime(t *testing.T) {
	assert.True(t, initTime(0).IsZero())
	assert.Equal(t, int64(1500), initTime(1500).UnixNano())
}
