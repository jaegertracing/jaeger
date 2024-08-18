// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ spanstore.Reader = (*archiveReader)(nil)
	_ spanstore.Writer = (*archiveWriter)(nil)
)

// archiveReader wraps storage_v1.ArchiveSpanReaderPluginClient into spanstore.Reader
type archiveReader struct {
	client storage_v1.ArchiveSpanReaderPluginClient
}

// ArchiveWriter wraps storage_v1.ArchiveSpanWriterPluginClient into spanstore.Writer
type archiveWriter struct {
	client storage_v1.ArchiveSpanWriterPluginClient
}

// GetTrace takes a traceID and returns a Trace associated with that traceID from Archive Storage
func (r *archiveReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	stream, err := r.client.GetArchiveTrace(upgradeContext(ctx), &storage_v1.GetTraceRequest{
		TraceID: traceID,
	})
	if status.Code(err) == codes.NotFound {
		return nil, spanstore.ErrTraceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return readTrace(stream)
}

// GetServices not used in archiveReader
func (*archiveReader) GetServices(context.Context) ([]string, error) {
	return nil, errors.New("GetServices not implemented")
}

// GetOperations not used in archiveReader
func (*archiveReader) GetOperations(context.Context, spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	return nil, errors.New("GetOperations not implemented")
}

// FindTraces not used in archiveReader
func (*archiveReader) FindTraces(context.Context, *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil, errors.New("FindTraces not implemented")
}

// FindTraceIDs not used in archiveReader
func (*archiveReader) FindTraceIDs(context.Context, *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return nil, errors.New("FindTraceIDs not implemented")
}

// WriteSpan saves the span into Archive Storage
func (w *archiveWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	_, err := w.client.WriteArchiveSpan(ctx, &storage_v1.WriteSpanRequest{
		Span: span,
	})
	if err != nil {
		return fmt.Errorf("plugin error: %w", err)
	}

	return nil
}
