// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
)

const (
	// dependencyType is the documentation type for the dependencies
	dependencyType          = "dependencies"
	dependencyIndexBaseName = "jaeger-dependencies"
)

// CoreDependencyStore is a DB Level abstraction which directly read/write dependencies into ElasticSearch
type CoreDependencyStore interface {
	// WriteDependencies write dependencies to Elasticsearch
	WriteDependencies(ts time.Time, dependencies []dbmodel.DependencyLink) error
	// CreateTemplates creates index templates.
	CreateTemplates(dependenciesTemplate string) error
	// GetDependencies returns all interservice dependencies
	GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]dbmodel.DependencyLink, error)
}

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	client                 func() es.Client
	logger                 *zap.Logger
	dependencyIndexPrefix  string
	indexDateLayout        string
	indexRolloverFrequency time.Duration
	maxDocCount            int
	useReadWriteAliases    bool
}

// Params holds the parameters for the DependencyStore
type Params struct {
	Client                 func() es.Client
	Logger                 *zap.Logger
	IndexPrefix            config.IndexPrefix
	IndexDateLayout        string
	IndexRolloverFrequency time.Duration
	MaxDocCount            int
	UseReadWriteAliases    bool
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(p Params) *DependencyStore {
	return &DependencyStore{
		client:                 p.Client,
		logger:                 p.Logger,
		dependencyIndexPrefix:  p.IndexPrefix.Apply(dependencyIndexBaseName) + config.IndexPrefixSeparator,
		indexDateLayout:        p.IndexDateLayout,
		indexRolloverFrequency: p.IndexRolloverFrequency,
		maxDocCount:            p.MaxDocCount,
		useReadWriteAliases:    p.UseReadWriteAliases,
	}
}

// WriteDependencies implements dependencyWriter
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []dbmodel.DependencyLink) error {
	indexName := s.getWriteIndex(ts)
	if err := s.createIndex(indexName); err != nil {
		return err
	}
	s.writeDependencies(indexName, ts, dependencies)
	return nil
}

// CreateTemplates creates index templates.
func (s *DependencyStore) CreateTemplates(dependenciesTemplate string) error {
	ctx := context.Background()
	_, err := s.client().CreateTemplate("jaeger-dependencies").Body(dependenciesTemplate).Do(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (s *DependencyStore) createIndex(indexName string) error {
	ctx := context.Background()
	if s.useReadWriteAliases {
		return nil
	}
	exists, err := s.client().IndexExists(indexName).Do(ctx)
	if err != nil {
		return err
	}
	if !exists {
		_, err := s.client().CreateIndex(indexName).Do(ctx)
		return err
	}
	return nil
}

func (s *DependencyStore) getWriteIndex(ts time.Time) string {
	if s.useReadWriteAliases {
		return s.dependencyIndexPrefix + "write"
	}
	return config.IndexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, ts)
}

func (s *DependencyStore) writeDependencies(indexName string, ts time.Time, dependencies []dbmodel.DependencyLink) {
	s.client().Index().Index(indexName).Type(dependencyType).
		BodyJson(&dbmodel.TimeDependencies{
			Timestamp:    ts,
			Dependencies: dependencies,
		}).Add("")
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]dbmodel.DependencyLink, error) {
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
	for _, hit := range searchResult.Hits.Hits {
		var tDependencies dbmodel.TimeDependencies
		if err := json.Unmarshal(hit.Source, &tDependencies); err != nil {
			return nil, fmt.Errorf("unmarshalling documents failed: %w", err)
		}
		retDependencies = append(retDependencies, tDependencies.Dependencies...)
	}
	return retDependencies, nil
}

func (s *DependencyStore) getReadIndices(endTs time.Time, lookback time.Duration) []string {
	if s.useReadWriteAliases {
		return []string{s.dependencyIndexPrefix + "read"}
	}
	var indices []string
	firstIndex := config.IndexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, endTs.Add(-lookback))
	currentIndex := config.IndexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, endTs)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		endTs = endTs.Add(-s.indexRolloverFrequency)
		currentIndex = config.IndexWithDate(s.dependencyIndexPrefix, s.indexDateLayout, endTs)
	}
	indices = append(indices, firstIndex)
	return indices
}

func buildTSQuery(endTs time.Time, lookback time.Duration) elastic.Query {
	return esquery.NewRangeQuery("timestamp").Gte(endTs.Add(-lookback)).Lte(endTs)
}
