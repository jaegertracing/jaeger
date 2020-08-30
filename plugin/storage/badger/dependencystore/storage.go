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

package dependencystore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// DependencyStore handles all queries and insertions to Cassandra dependencies
type DependencyStore struct {
	reader spanstore.Reader
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(store spanstore.Reader) *DependencyStore {
	return &DependencyStore{
		reader: store,
	}
}

// GetDependencies returns all interservice dependencies, implements DependencyReader
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	deps := map[string]*model.DependencyLink{}

	params := &spanstore.TraceQueryParameters{
		StartTimeMin: endTs.Add(-1 * lookback),
		StartTimeMax: endTs,
	}

	// We need to do a full table scan - if this becomes a bottleneck, we can write an index that describes
	// dependencyKeyPrefix + timestamp + parent + child key and do a key-only seek (which is fast - but requires additional writes)

	// GetDependencies is not shipped with a context like the SpanReader / SpanWriter
	traces, err := s.reader.FindTraces(context.Background(), params)
	if err != nil {
		return nil, err
	}
	for _, tr := range traces {
		processTrace(deps, tr)
	}

	return depMapToSlice(deps), err
}

// depMapToSlice modifies the spans to DependencyLink in the same way as the memory storage plugin
func depMapToSlice(deps map[string]*model.DependencyLink) []model.DependencyLink {
	retMe := make([]model.DependencyLink, 0, len(deps))
	for _, dep := range deps {
		retMe = append(retMe, *dep)
	}
	return retMe
}

// processTrace is copy from the memory storage plugin
func processTrace(deps map[string]*model.DependencyLink, trace *model.Trace) {
	for _, s := range trace.Spans {
		parentSpan := seekToSpan(trace, s.ParentSpanID())
		if parentSpan != nil {
			if parentSpan.Process.ServiceName == s.Process.ServiceName {
				continue
			}
			depKey := parentSpan.Process.ServiceName + "&&&" + s.Process.ServiceName
			if _, ok := deps[depKey]; !ok {
				deps[depKey] = &model.DependencyLink{
					Parent:    parentSpan.Process.ServiceName,
					Child:     s.Process.ServiceName,
					CallCount: 1,
				}
			} else {
				deps[depKey].CallCount++
			}
		}
	}
}

func seekToSpan(trace *model.Trace, spanID model.SpanID) *model.Span {
	for _, s := range trace.Spans {
		if s.SpanID == spanID {
			return s
		}
	}
	return nil
}
