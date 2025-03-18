// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
)

// CoreSpanReader is a DB-Level abstraction which directly deals with database level operations
type CoreSpanReader interface {
	// FindTraceIDs retrieves traces IDs that match the traceQuery
	FindTraceIDs(ctx context.Context, traceQuery *dbmodel.TraceQueryParameters) ([]dbmodel.TraceID, error)
	// FindTraces retrieves traces that match the traceQuery
	FindTraces(ctx context.Context, traceQuery *dbmodel.TraceQueryParameters) ([]*dbmodel.Trace, error)
	// GetOperations returns all operations for a specific service traced by Jaeger
	GetOperations(ctx context.Context, query dbmodel.OperationQueryParameters) ([]dbmodel.Operation, error)
	// GetServices returns all services traced by Jaeger, ordered by frequency
	GetServices(ctx context.Context) ([]string, error)
	// GetTraces takes a traceID and returns a Trace associated with that traceID
	GetTraces(ctx context.Context, query []dbmodel.TraceID) ([]*dbmodel.Trace, error)
}
