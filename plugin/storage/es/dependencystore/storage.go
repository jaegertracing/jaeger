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
	Timestamp    time.Time              `json:"timestamp"`
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
	return s.writeDependencies(indexName, ts, dependencies)
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
			Timestamp:    ts,
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
		IgnoreUnavailable(true).
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
	firstIndex := indexName(ts.Add(-lookback))
	currentIndex := indexName(ts)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		ts = ts.Add(-24 * time.Hour)
		currentIndex = indexName(ts)
	}
	return append(indices, firstIndex)
}

func indexName(date time.Time) string {
	return dependencyIndexPrefix + date.UTC().Format("2006-01-02")
}
