// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package blackhole

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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
func (st *Store) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return []model.DependencyLink{}, nil
}

// WriteSpan writes the given span to blackhole.
func (st *Store) WriteSpan(ctx context.Context, span *model.Span) error {
	return nil
}

// GetTrace gets nothing.
func (st *Store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return nil, spanstore.ErrTraceNotFound
}

// GetServices returns nothing.
func (st *Store) GetServices(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

// GetOperations returns nothing.
func (st *Store) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return []spanstore.Operation{}, nil
}

// FindTraces returns nothing.
func (st *Store) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return []*model.Trace{}, nil
}

// FindTraceIDs returns nothing.
func (m *Store) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return []model.TraceID{}, nil
}
