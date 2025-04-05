// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var (
	_ storage.TraceReaderServer      = (*Server)(nil)
	_ storage.DependencyReaderServer = (*Server)(nil)
	_ ptraceotlp.GRPCServer          = (*Server)(nil)
)

type Server struct {
	storage.UnimplementedTraceReaderServer
	storage.UnimplementedDependencyReaderServer
	ptraceotlp.UnimplementedGRPCServer

	traceReader tracestore.Reader
}

func NewServer(traceReader tracestore.Reader) *Server {
	return &Server{
		traceReader: traceReader,
	}
}

func (h *Server) GetServices(
	ctx context.Context,
	_ *storage.GetServicesRequest,
) (*storage.GetServicesResponse, error) {
	services, err := h.traceReader.GetServices(ctx)
	if err != nil {
		return nil, err
	}
	return &storage.GetServicesResponse{
		Services: services,
	}, nil
}
