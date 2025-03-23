// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testWriterServer struct {
	ptraceotlp.UnimplementedGRPCServer

	err error
}

func (s *testWriterServer) Export(
	context.Context,
	ptraceotlp.ExportRequest,
) (ptraceotlp.ExportResponse, error) {
	return ptraceotlp.NewExportResponse(), s.err
}

func startWriterServer(t *testing.T, testServer *testWriterServer) *grpc.ClientConn {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := grpc.NewServer()
	ptraceotlp.RegisterGRPCServer(server, testServer)

	go func() {
		server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			conn.Close()
			server.Stop()
			listener.Close()
		},
	)

	return conn
}

func TestTraceWriter_WriteTraces(t *testing.T) {
	tests := []struct {
		name        string
		serverErr   error
		expectedErr string
	}{
		{
			name: "no error",
		},
		{
			name:        "server error",
			serverErr:   assert.AnError,
			expectedErr: "failed to export traces",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &testWriterServer{err: test.serverErr}
			conn := startWriterServer(t, server)

			writer := NewTraceWriter(conn)
			err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.expectedErr)
			}
		})
	}
}
