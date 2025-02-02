// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package blackhole

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/spanstore"
)

// Store is a blackhole. It creates an artificial micro-singularity
// and forwards all writes to it. We do not know what happens to the
// data once it reaches the singulatiry, but we know that we cannot
// get it back.
type Store struct {
	// nothing, just darkness
}

// NewStore creates a blackhole store.
func NewStore() *Store {
	return &Store{}
}

// GetDependencies returns nothing.
func (*Store) GetDependencies(context.Context, time.Time /* endTs */, time.Duration /* lookback */) ([]model.DependencyLink, error) {
	return []model.DependencyLink{}, nil
}

// WriteSpan writes the given span to blackhole.
func (*Store) WriteSpan(context.Context, *model.Span) error {
	return nil
}

// GetTrace gets nothing.
func (*Store) GetTrace(context.Context, spanstore.GetTraceParameters) (*model.Trace, error) {
	return nil, spanstore.ErrTraceNotFound
}

// GetServices returns nothing.
func (*Store) GetServices(context.Context) ([]string, error) {
	return []string{}, nil
}

// GetOperations returns nothing.
func (*Store) GetOperations(
	context.Context,
	spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return []spanstore.Operation{}, nil
}

// FindTraces returns nothing.
func (*Store) FindTraces(context.Context, *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return []*model.Trace{}, nil
}

// FindTraceIDs returns nothing.
func (*Store) FindTraceIDs(context.Context, *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return []model.TraceID{}, nil
}
