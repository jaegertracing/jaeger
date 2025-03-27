// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/olivere/elastic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/dependencystore/dbmodel"
)

const (
	dependencyType          = "dependencies"
	dependencyIndexBaseName = "jaeger-dependencies-"
)

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	client                func() es.Client
	logger                *zap.Logger
	dependencyIndexPrefix string
	indexDateLayout       string
	maxDocCount           int
	useReadWriteAliases   bool
}

// DependencyStoreParams holds constructor parameters for NewDependencyStore
type Params struct {
	Client              func() es.Client
	Logger              *zap.Logger
	IndexPrefix         config.IndexPrefix
	IndexDateLayout     string
	MaxDocCount         int
	UseReadWriteAliases bool
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(p Params) *DependencyStore {
	return &DependencyStore{
		client:                p.Client,
		logger:                p.Logger,
		dependencyIndexPrefix: p.IndexPrefix.Apply(dependencyIndexBaseName),
		indexDateLayout:       p.IndexDateLayout,
		maxDocCount:           p.MaxDocCount,
		useReadWriteAliases:   p.UseReadWriteAliases,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	writeIndexName := s.getWriteIndex(ts)
	s.writeDependencies(writeIndexName, ts, dependencies)
	return nil
}

// CreateTemplates creates index templates.
func (s *DependencyStore) CreateTemplates(dependenciesTemplate string) error {
	_, err := s.client().CreateTemplate("jaeger-dependencies").Body(dependenciesTemplate).Do(context.Background())
	if err != nil {
		return err
	}
	return nil
}

func (s *DependencyStore) writeDependencies(indexName string, ts time.Time, dependencies []model.DependencyLink) {
	s.client().Index().Index(indexName).Type(dependencyType).
		BodyJson(&dbmodel.TimeDependencies{
			Timestamp:    ts,
			Dependencies: dbmodel.FromDomainDependencies(dependencies),
		}).Add()
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	indices := s.getReadIndices(endTs, lookback)
	searchResult, err := s.client().Search(indices...).
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

func (s *DependencyStore) getReadIndices(ts time.Time, lookback time.Duration) []string {
	if s.useReadWriteAliases {
		return []string{s.dependencyIndexPrefix + "read"}
	}
	var indices []string
	firstIndex := indexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, ts.Add(-lookback))
	currentIndex := indexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, ts)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		ts = ts.Add(-24 * time.Hour)
		currentIndex = indexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, ts)
	}
	return append(indices, firstIndex)
}

func indexWithDate(indexNamePrefix, indexDateLayout string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format(indexDateLayout)
}

func (s *DependencyStore) getWriteIndex(ts time.Time) string {
	if s.useReadWriteAliases {
		return s.dependencyIndexPrefix + "write"
	}
	return indexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, ts)
}
