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

package grpc

import (
	"context"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// ArchiveWriter implements spanstore.Writer
type ArchiveWriter struct {
	impl shared.ArchiveWriter
}

// WriteSpan saves the span
func (w *ArchiveWriter) WriteSpan(span *model.Span) error {
	return w.impl.WriteArchiveSpan(span)
}

// ArchiveReader implements spanstore.Reader
type ArchiveReader struct {
	impl shared.ArchiveReader
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (r *ArchiveReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return r.impl.GetArchiveTrace(ctx, traceID)
}

// GetServices is not used by archive storage
func (r *ArchiveReader) GetServices(ctx context.Context) ([]string, error) {
	panic("not implemented")
}

// GetOperations is not used by archive storage
func (r *ArchiveReader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	panic("not implemented")
}

// FindTraces is not used by archive storage
func (r *ArchiveReader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	panic("not implemented")
}

// FindTraceIDs is not used by archive storage
func (r *ArchiveReader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	panic("not implemented")
}
