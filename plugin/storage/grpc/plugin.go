// Copyright (c) 2018 The Jaeger Authors.
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
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/grpc/config"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type Store struct {
	config config.Configuration

	plugin shared.StoragePlugin
}

func (s *Store) WriteSpan(span *model.Span) error {
	return s.plugin.WriteSpan(span)
}

func (s *Store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return s.plugin.GetTrace(ctx, traceID)
}

func (s *Store) GetServices(ctx context.Context) ([]string, error) {
	return s.plugin.GetServices(ctx)
}

func (s *Store) GetOperations(ctx context.Context, service string) ([]string, error) {
	return s.plugin.GetOperations(ctx, service)
}

func (s *Store) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return s.plugin.FindTraces(ctx, query)
}

func (s *Store) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return s.plugin.FindTraceIDs(ctx, query)
}

func (s *Store) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return s.plugin.GetDependencies(endTs, lookback)
}
