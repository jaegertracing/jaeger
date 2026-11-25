// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type partialWriteTestServer struct {
	ptraceotlp.UnimplementedGRPCServer

	rejectedSpans int64
	message       string
	err           error
	calls         atomic.Int32
}

func (s *partialWriteTestServer) Export(
	context.Context,
	ptraceotlp.ExportRequest,
) (ptraceotlp.ExportResponse, error) {
	s.calls.Add(1)

	response := ptraceotlp.NewExportResponse()
	if s.rejectedSpans != 0 || s.message != "" {
		response.PartialSuccess().SetRejectedSpans(s.rejectedSpans)
		response.PartialSuccess().SetErrorMessage(s.message)
	}

	return response, s.err
}

func startPartialWriteServer(
	t *testing.T,
	backend ptraceotlp.GRPCServer,
) *grpc.ClientConn {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	ptraceotlp.RegisterGRPCServer(server, backend)

	go func() {
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		server.Stop()
		require.NoError(t, listener.Close())
	})

	conn, err := grpc.NewClient(
		"passthrough:///partial-write-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			},
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	return conn
}

func partialWriteTestTraces() ptrace.Traces {
	traces := makeTestTrace()
	spans := traces.ResourceSpans().
		At(0).
		ScopeSpans().
		At(0).
		Spans()

	spans.At(0).CopyTo(spans.AppendEmpty())
	spans.At(1).SetSpanID(pcommon.SpanID{7: 3})

	return traces
}

func TestTraceWriter_RemoteWriteResponse(t *testing.T) {
	tests := []struct {
		name          string
		rejectedSpans int64
		message       string
		rpcErr        error
	}{
		{
			name: "full success",
		},
		{
			name:    "warning only",
			message: "consider reducing batch size",
		},
		{
			name:          "partial rejection",
			rejectedSpans: 1,
			message:       "invalid attribute",
		},
		{
			name:          "rejection without explanation",
			rejectedSpans: 1,
		},
		{
			name:   "RPC failure",
			rpcErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &partialWriteTestServer{
				rejectedSpans: test.rejectedSpans,
				message:       test.message,
				err:           test.rpcErr,
			}
			writer := NewTraceWriter(
				startPartialWriteServer(t, backend),
			)

			ctx, cancel := context.WithTimeout(
				t.Context(),
				5*time.Second,
			)
			defer cancel()

			err := writer.WriteTraces(ctx, partialWriteTestTraces())

			if test.rpcErr != nil {
				require.ErrorContains(
					t,
					err,
					"failed to export traces",
				)
				assert.False(t, consumererror.IsPermanent(err))
			} else if test.rejectedSpans > 0 {
				require.Error(t, err)
				assert.True(t, consumererror.IsPermanent(err))

				var partial *partialWriteError
				require.ErrorAs(t, err, &partial)

				assert.Equal(
					t,
					test.rejectedSpans,
					partial.rejectedSpans,
				)
				assert.Equal(t, test.message, partial.message)

				expected := "remote storage rejected spans (count=1)"
				if test.message != "" {
					expected += ": " + test.message
				}
				assert.EqualError(t, partial, expected)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, int32(1), backend.calls.Load())
		})
	}
}

func TestTraceWriter_PartialRejectionDoesNotRetry(t *testing.T) {
	backend := &partialWriteTestServer{
		rejectedSpans: 1,
		message:       "invalid attribute",
	}
	writer := NewTraceWriter(startPartialWriteServer(t, backend))

	retry := configretry.NewDefaultBackOffConfig()
	retry.Enabled = true
	retry.InitialInterval = time.Millisecond
	retry.MaxInterval = time.Millisecond
	retry.MaxElapsedTime = time.Second

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	exporter, err := exporterhelper.NewTraces(
		ctx,
		exportertest.NewNopSettings(exportertest.NopType),
		&retry,
		writer.WriteTraces,
		exporterhelper.WithRetry(retry),
	)
	require.NoError(t, err)
	require.NoError(t, exporter.Start(ctx, componenttest.NewNopHost()))

	t.Cleanup(func() {
		require.NoError(t, exporter.Shutdown(t.Context()))
	})

	err = exporter.ConsumeTraces(ctx, partialWriteTestTraces())

	require.Error(t, err)
	assert.True(t, consumererror.IsPermanent(err))
	assert.Equal(t, int32(1), backend.calls.Load())
}

func TestTraceWriter_PartialRejectionThroughRemoteStorage(
	t *testing.T,
) {
	backend := &partialWriteTestServer{
		rejectedSpans: 1,
		message:       "invalid attribute",
	}

	backendWriter := NewTraceWriter(
		startPartialWriteServer(t, backend),
	)
	proxy := NewHandler(nil, backendWriter, nil)
	proxyConn := startPartialWriteServer(t, proxy)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	response, err := ptraceotlp.NewGRPCClient(proxyConn).Export(
		ctx,
		ptraceotlp.NewExportRequestFromTraces(
			partialWriteTestTraces(),
		),
	)

	require.NoError(t, err)
	assert.Equal(
		t,
		int64(1),
		response.PartialSuccess().RejectedSpans(),
	)
	assert.Equal(
		t,
		"invalid attribute",
		response.PartialSuccess().ErrorMessage(),
	)
	assert.Equal(t, int32(1), backend.calls.Load())
}
