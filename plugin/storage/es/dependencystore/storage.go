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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/olivere/elastic"
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
	client          es.Client
	logger          *zap.Logger
	indexPrefix     string
	indexDateLayout string
	maxDocCount     int
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(client es.Client, logger *zap.Logger, indexPrefix, indexDateLayout string, maxDocCount int) *DependencyStore {
	var prefix string
	if indexPrefix != "" {
		prefix = indexPrefix + "-"
	}
	return &DependencyStore{
		client:          client,
		logger:          logger,
		indexPrefix:     prefix + dependencyIndex,
		indexDateLayout: indexDateLayout,
		maxDocCount:     maxDocCount,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	indexName := indexWithDate(s.indexPrefix, s.indexDateLayout, ts)
	s.writeDependencies(indexName, ts, dependencies)
	return nil
}

// CreateTemplates creates index templates.
func (s *DependencyStore) CreateTemplates(dependenciesTemplate string) error {
	_, err := s.client.CreateTemplate("jaeger-dependencies").Body(dependenciesTemplate).Do(context.Background())
	if err != nil {
		return err
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
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	indices := getIndices(s.indexPrefix, s.indexDateLayout, endTs, lookback)
	searchResult, err := s.client.Search(indices...).
		Size(s.maxDocCount).
		Query(buildTSQuery(endTs, lookback)).
		IgnoreUnavailable(true).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to search for dependencies: %w", err)
	}

	var retDependencies []dbmodel.DependencyLink
	hits := searchResult.Hits.Hits
	for _, hit := range hits {
		source := hit.Source
		var tToD dbmodel.TimeDependencies
		if err := json.Unmarshal(*source, &tToD); err != nil {
			return nil, errors.New("unmarshalling ElasticSearch documents failed")
		}
		retDependencies = append(retDependencies, tToD.Dependencies...)
	}
	return dbmodel.ToDomainDependencies(retDependencies), nil
}

func buildTSQuery(endTs time.Time, lookback time.Duration) elastic.Query {
	return elastic.NewRangeQuery("timestamp").Gte(endTs.Add(-lookback)).Lte(endTs)
}

func getIndices(prefix, dateLayout string, ts time.Time, lookback time.Duration) []string {
	var indices []string
	firstIndex := indexWithDate(prefix, dateLayout, ts.Add(-lookback))
	currentIndex := indexWithDate(prefix, dateLayout, ts)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		ts = ts.Add(-24 * time.Hour)
		currentIndex = indexWithDate(prefix, dateLayout, ts)
	}
	return append(indices, firstIndex)
}

func indexWithDate(indexNamePrefix, indexDateLayout string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format(indexDateLayout)
}
