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

package dependencystore

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
)

const (
	depsInsertStmt = "INSERT INTO dependencies(ts, ts_index, dependencies) VALUES (?, ?, ?)"
	depsSelectStmt = "SELECT ts, dependencies FROM dependencies WHERE ts_index >= ? AND ts_index < ?"
)

// DependencyStore handles all queries and insertions to Cassandra dependencies
type DependencyStore struct {
	session                  cassandra.Session
	dependenciesTableMetrics *casMetrics.Table
	logger                   *zap.Logger
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(
	session cassandra.Session,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *DependencyStore {
	return &DependencyStore{
		session:                  session,
		dependenciesTableMetrics: casMetrics.NewTable(metricsFactory, "dependencies"),
		logger: logger,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	deps := make([]Dependency, len(dependencies))
	for i, d := range dependencies {
		deps[i] = Dependency{
			Parent:    d.Parent,
			Child:     d.Child,
			CallCount: int64(d.CallCount),
		}
	}
	query := s.session.Query(depsInsertStmt, ts, ts, deps)
	return s.dependenciesTableMetrics.Exec(query, s.logger)
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	query := s.session.Query(depsSelectStmt, endTs.Add(-1*lookback), endTs)
	iter := query.Consistency(cassandra.One).Iter()

	var mDependency []model.DependencyLink
	var dependencies []Dependency
	var ts time.Time
	for iter.Scan(&ts, &dependencies) {
		for _, dependency := range dependencies {
			mDependency = append(mDependency, model.DependencyLink{
				Parent:    dependency.Parent,
				Child:     dependency.Child,
				CallCount: uint64(dependency.CallCount),
			})
		}
	}

	if err := iter.Close(); err != nil {
		s.logger.Error("Failure to read Dependencies", zap.Time("endTs", endTs), zap.Duration("lookback", lookback), zap.Error(err))
		return nil, errors.Wrap(err, "Error reading dependencies from storage")
	}
	return mDependency, nil
}
