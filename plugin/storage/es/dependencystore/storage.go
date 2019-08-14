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

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore/dbmodel"
)

const (
	dependencyType  = "dependencies"
	dependencyIndex = "jaeger-dependencies-"
)

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	ctx         context.Context
	client      es.Client
	logger      *zap.Logger
	indexPrefix string
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(client es.Client, logger *zap.Logger, indexPrefix string) *DependencyStore {
	var prefix string
	if indexPrefix != "" {
		prefix = indexPrefix + "-"
	}
	return &DependencyStore{
		ctx:         context.Background(),
		client:      client,
		logger:      logger,
		indexPrefix: prefix + dependencyIndex,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	indexName := indexWithDate(s.indexPrefix, ts)
	if err := s.createIndex(indexName); err != nil {
		return err
	}
	s.writeDependencies(indexName, ts, dependencies)
	return nil
}

func (s *DependencyStore) createIndex(indexName string) error {
	_, err := s.client.CreateIndex(indexName).Body(getMapping(s.client.GetVersion())).Do(s.ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to create index")
	}
	return nil
}

func (s *DependencyStore) writeDependencies(indexName string, ts time.Time, dependencies []model.DependencyLink) {
	s.client.Index().Index(indexName).Type(dependencyType).
		BodyJson(&dbmodel.TimeDependencies{Timestamp: ts,
			Dependencies: dbmodel.FromDomainDependencies(dependencies),
		}).Add()
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	indices := getIndices(s.indexPrefix, endTs, lookback)
	searchResult, err := s.client.Search(indices...).
		Size(10000). // the default elasticsearch allowed limit
		Query(buildTSQuery(endTs, lookback)).
		IgnoreUnavailable(true).
		Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to search for dependencies")
	}

	var retDependencies []dbmodel.DependencyLink
	hits := searchResult.Hits.Hits
	for _, hit := range hits {
		source := hit.Source
		var tToD dbmodel.TimeDependencies
		if err := json.Unmarshal(*source, &tToD); err != nil {
			return nil, errors.New("Unmarshalling ElasticSearch documents failed")
		}
		retDependencies = append(retDependencies, tToD.Dependencies...)
	}
	return dbmodel.ToDomainDependencies(retDependencies), nil
}

func buildTSQuery(endTs time.Time, lookback time.Duration) elastic.Query {
	return elastic.NewRangeQuery("timestamp").Gte(endTs.Add(-lookback)).Lte(endTs)
}

func getIndices(prefix string, ts time.Time, lookback time.Duration) []string {
	var indices []string
	firstIndex := indexWithDate(prefix, ts.Add(-lookback))
	currentIndex := indexWithDate(prefix, ts)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		ts = ts.Add(-24 * time.Hour)
		currentIndex = indexWithDate(prefix, ts)
	}
	return append(indices, firstIndex)
}

func indexWithDate(indexNamePrefix string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format("2006-01-02")
}
