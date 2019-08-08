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
	"time"

	"github.com/jaegertracing/jaeger/model"
	badgerStore "github.com/jaegertracing/jaeger/plugin/storage/badger/spanstore"
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
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	br := s.reader.(*badgerStore.TraceReader)
	query := &spanstore.TraceQueryParameters{
		StartTimeMax: endTs,
		StartTimeMin: endTs.Add(-1 * lookback),
	}
	resultMap, err := br.ScanDependencyIndex(query)
	if err != nil {
		return nil, err
	}

	retMe := make([]model.DependencyLink, 0, len(resultMap))

	for k, v := range resultMap {
		retMe = append(retMe, model.DependencyLink{
			Parent:    k.From,
			Child:     k.To,
			CallCount: v,
		})
	}

	return retMe, nil
}
