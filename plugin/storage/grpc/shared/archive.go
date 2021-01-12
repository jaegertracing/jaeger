// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	stream, err := r.client.GetArchiveTrace(upgradeContextWithBearerToken(ctx), &storage_v1.GetTraceRequest{
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
func (r *archiveReader) GetServices(ctx context.Context) ([]string, error) {
	return nil, errors.New("GetServices not implemented")
}

// GetOperations not used in archiveReader
func (r *archiveReader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	return nil, errors.New("GetOperations not implemented")
}

// FindTraces not used in archiveReader
func (r *archiveReader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil, errors.New("FindTraces not implemented")
}

// FindTraceIDs not used in archiveReader
func (r *archiveReader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
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
