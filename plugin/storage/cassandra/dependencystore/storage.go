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

package dependencystore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
)

// Version determines which version of the dependencies table to use.
type Version int

// IsValid returns true if the Version is a valid one.
func (i Version) IsValid() bool {
	return i >= 0 && i < versionEnumEnd
}

const (
	// V1 is used when the dependency table is SASI indexed.
	V1 Version = iota

	// V2 is used when the dependency table is NOT SASI indexed.
	V2
	versionEnumEnd

	depsInsertStmtV1 = "INSERT INTO dependencies(ts, ts_index, dependencies) VALUES (?, ?, ?)"
	depsInsertStmtV2 = "INSERT INTO dependencies_v2(ts, ts_bucket, dependencies) VALUES (?, ?, ?)"
	depsSelectStmtV1 = "SELECT ts, dependencies FROM dependencies WHERE ts_index >= ? AND ts_index < ?"
	depsSelectStmtV2 = "SELECT ts, dependencies FROM dependencies_v2 WHERE ts_bucket IN ? AND ts >= ? AND ts < ?"

	// TODO: Make this customizable.
	tsBucket = 24 * time.Hour
)

var (
	errInvalidVersion = errors.New("invalid version")
)

// DependencyStore handles all queries and insertions to Cassandra dependencies
type DependencyStore struct {
	session                  cassandra.Session
	dependenciesTableMetrics *casMetrics.Table
	logger                   *zap.Logger
	version                  Version
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(
	session cassandra.Session,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	version Version,
) (*DependencyStore, error) {
	if !version.IsValid() {
		return nil, errInvalidVersion
	}
	return &DependencyStore{
		session:                  session,
		dependenciesTableMetrics: casMetrics.NewTable(metricsFactory, "dependencies"),
		logger:                   logger,
		version:                  version,
	}, nil
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	deps := make([]Dependency, len(dependencies))
	for i, d := range dependencies {
		deps[i] = Dependency{
			Parent:    d.Parent,
			Child:     d.Child,
			CallCount: int64(d.CallCount),
			Source:    string(d.Source),
		}
	}

	var query cassandra.Query
	switch s.version {
	case V1:
		query = s.session.Query(depsInsertStmtV1, ts, ts, deps)
	case V2:
		query = s.session.Query(depsInsertStmtV2, ts, ts.Truncate(tsBucket), deps)
	}
	return s.dependenciesTableMetrics.Exec(query, s.logger)
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	startTs := endTs.Add(-1 * lookback)
	var query cassandra.Query
	switch s.version {
	case V1:
		query = s.session.Query(depsSelectStmtV1, startTs, endTs)
	case V2:
		query = s.session.Query(depsSelectStmtV2, getBuckets(startTs, endTs), startTs, endTs)
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
				Source:    dependency.Source,
			}.ApplyDefaults()
			mDependency = append(mDependency, dl)
		}
	}

	if err := iter.Close(); err != nil {
		s.logger.Error("Failure to read Dependencies", zap.Time("endTs", endTs), zap.Duration("lookback", lookback), zap.Error(err))
		return nil, fmt.Errorf("error reading dependencies from storage: %w", err)
	}
	return mDependency, nil
}

func getBuckets(startTs time.Time, endTs time.Time) []time.Time {
	// TODO: Preallocate the array using some maths and maybe use a pool? This endpoint probably isn't used enough to warrant this.
	var tsBuckets []time.Time
	for ts := startTs.Truncate(tsBucket); ts.Before(endTs); ts = ts.Add(tsBucket) {
		tsBuckets = append(tsBuckets, ts)
	}
	return tsBuckets
}
