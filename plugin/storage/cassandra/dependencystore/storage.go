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
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
)

// IndexMode determines how the dependency data is indexed.
type IndexMode int

const (
	// SASIEnabled is used when the dependency table is SASI indexed.
	SASIEnabled IndexMode = iota

	// SASIDisabled is used when the dependency table is NOT SASI indexed.
	SASIDisabled

	depsInsertStmtSASI = "INSERT INTO dependencies(ts, ts_index, dependencies) VALUES (?, ?, ?)"
	depsInsertStmt     = "INSERT INTO dependenciesv2(ts, date_bucket, dependencies) VALUES (?, ?, ?)"
	depsSelectStmtSASI = "SELECT ts, dependencies FROM dependencies WHERE ts_index >= ? AND ts_index < ?"
	depsSelectFmt      = "SELECT ts, dependencies FROM dependenciesv2 WHERE date_bucket IN (%s) AND ts >= ? AND ts < ?"
	dateFmt            = "20060102"
	day                = 24 * time.Hour
)

// DependencyStore handles all queries and insertions to Cassandra dependencies
type DependencyStore struct {
	session                  cassandra.Session
	dependenciesTableMetrics *casMetrics.Table
	logger                   *zap.Logger
	indexMode                IndexMode
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(
	session cassandra.Session,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	indexMode IndexMode,
) *DependencyStore {
	return &DependencyStore{
		session:                  session,
		dependenciesTableMetrics: casMetrics.NewTable(metricsFactory, "dependencies"),
		logger:                   logger,
		indexMode:                indexMode,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	deps := make([]Dependency, len(dependencies))
	for i, d := range dependencies {
		dep := Dependency{
			Parent:    d.Parent,
			Child:     d.Child,
			CallCount: int64(d.CallCount),
		}
		if s.indexMode == SASIEnabled {
			dep.Source = string(d.Source)
		}
		deps[i] = dep
	}

	var query cassandra.Query
	switch s.indexMode {
	case SASIDisabled:
		query = s.session.Query(depsInsertStmt, ts, ts.Format(dateFmt), deps)
	case SASIEnabled:
		query = s.session.Query(depsInsertStmtSASI, ts, ts, deps)
	}
	return s.dependenciesTableMetrics.Exec(query, s.logger)
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	startTs := endTs.Add(-1 * lookback)
	var query cassandra.Query
	switch s.indexMode {
	case SASIDisabled:
		query = s.session.Query(getDepSelectString(startTs, endTs), startTs, endTs)
	case SASIEnabled:
		query = s.session.Query(depsSelectStmtSASI, startTs, endTs)
	}
	iter := query.Consistency(cassandra.One).Iter()

	var mDependency []model.DependencyLink
	var dependencies []Dependency
	var ts time.Time
	for iter.Scan(&ts, &dependencies) {
		for _, dependency := range dependencies {
			dl := model.DependencyLink{
				Parent:    dependency.Parent,
				Child:     dependency.Child,
				CallCount: uint64(dependency.CallCount),
				Source:    model.DependencyLinkSource(dependency.Source),
			}.Sanitize()
			mDependency = append(mDependency, dl)
		}
	}

	if err := iter.Close(); err != nil {
		s.logger.Error("Failure to read Dependencies", zap.Time("endTs", endTs), zap.Duration("lookback", lookback), zap.Error(err))
		return nil, errors.Wrap(err, "Error reading dependencies from storage")
	}
	return mDependency, nil
}

func getDepSelectString(startTs time.Time, endTs time.Time) string {
	var dateBuckets []string
	for ts := startTs.Truncate(day); ts.Before(endTs); ts = ts.Add(day) {
		dateBuckets = append(dateBuckets, ts.Format(dateFmt))
	}
	return fmt.Sprintf(depsSelectFmt, strings.Join(dateBuckets, ","))
}
