// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dependencystore

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
)

const (
	depsInsertStmt = "INSERT INTO dependencies(ts, ts_index, dependencies) VALUES (?, ?, ?)"
	depsSelectStmt = "SELECT ts, dependencies FROM dependencies where ts_index >= ? AND ts_index < ?"
)

// DependencyStore handles all queries and insertions to Cassandra dependencies
type DependencyStore struct {
	session                  cassandra.Session
	dependencyDataFrequency  time.Duration
	dependenciesTableMetrics *casMetrics.Table
	logger                   zap.Logger
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(
	session cassandra.Session,
	dependencyDataFrequency time.Duration,
	metricsFactory metrics.Factory,
	logger zap.Logger,
) *DependencyStore {
	return &DependencyStore{
		session:                  session,
		dependencyDataFrequency:  dependencyDataFrequency,
		dependenciesTableMetrics: casMetrics.NewTable(metricsFactory, "Dependencies"),
		logger: logger,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	s.logger.Info("Saving dependencies", zap.Time("timestamp", ts))
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

func (s *DependencyStore) timeIntervalToPoints(endTs time.Time, lookback time.Duration) []time.Time {
	startTs := endTs.Add(-lookback)
	var days []time.Time
	for day := endTs; startTs.Before(day); day = day.Add(-s.dependencyDataFrequency) {
		days = append(days, day.Truncate(s.dependencyDataFrequency))
	}
	return days
}
