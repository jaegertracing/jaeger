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
	"context"
	"encoding/json"
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es"
)

const (
	dependencyType        = "dependencies"
	dependencyIndexPrefix = "jaeger-dependencies-"
)

type timeToDependencies struct {
	Timestamp    time.Time                 `json:"timestamp"`
	Dependencies []model.DependencyLink `json:"dependencies"`
}

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	ctx    context.Context
	client es.Client
	logger *zap.Logger
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(client es.Client, logger *zap.Logger) *DependencyStore {
	return &DependencyStore{
		ctx:    context.Background(),
		client: client,
		logger: logger,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	indexName := indexName(ts)
	if err := s.createIndex(indexName); err != nil {
		return err
	}
	if err := s.writeDependencies(indexName, ts, dependencies); err != nil {
		return err
	}
	return nil
}

func (s *DependencyStore) createIndex(indexName string) error {
	_, err := s.client.CreateIndex(indexName).Body(dependenciesMapping).Do(s.ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to create index")
	}
	return nil
}

func (s *DependencyStore) writeDependencies(indexName string, ts time.Time, dependencies []model.DependencyLink) error {
	_, err := s.client.Index().Index(indexName).
		Type(dependencyType).
		BodyJson(&timeToDependencies{
			Timestamp: ts,
			Dependencies: dependencies,
		}).
		Do(s.ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to write dependencies")
	}
	return nil
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	searchResult, err := s.client.Search(getIndices(endTs, lookback)...).
		Type(dependencyType).
		Size(10000). // the default elasticsearch allowed limit
		Query(buildTSQuery(endTs, lookback)).
		Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to search for dependencies")
	}

	var retDependencies []model.DependencyLink
	hits := searchResult.Hits.Hits
	for _, hit := range hits {
		source := hit.Source
		var tToD timeToDependencies
		if err := json.Unmarshal(*source, &tToD); err != nil {
			return nil, errors.New("Unmarshalling ElasticSearch documents failed")
		}
		retDependencies = append(retDependencies, tToD.Dependencies...)
	}
	return retDependencies, nil
}

func buildTSQuery(endTs time.Time, lookback time.Duration) elastic.Query {
	return elastic.NewRangeQuery("timestamp").Gte(endTs.Add(-lookback)).Lte(endTs)
}

func getIndices(ts time.Time, lookback time.Duration) []string {
	var indices []string

	// first add current date to indices, then round down to midnight
	indices = append(indices, indexName(ts))
	tsRoundedDown := ts.UTC().Truncate(24*time.Hour)
	lookback = lookback - ts.Sub(tsRoundedDown)

	// then add any dates previous that fit into the lookback scope
	for lookback >= 0 {
		tsRoundedDown = tsRoundedDown.Add(-24 * time.Hour)
		indices = append(indices, indexName(tsRoundedDown))
		lookback -= 24 * time.Hour
	}
	return indices
}

func indexName(date time.Time) string {
	return dependencyIndexPrefix + date.UTC().Format("2006-01-02")
}
