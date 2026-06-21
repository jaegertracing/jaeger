// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
)

const dependencyType = "dependencies"

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
	client      func() es.Client
	logger      *zap.Logger
	maxDocCount int
	rotation    indices.Rotation
}

// Params holds constructor parameters for NewDependencyStore
type Params struct {
	Client      func() es.Client
	Logger      *zap.Logger
	MaxDocCount int
	Rotation    indices.Rotation
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(p Params) *DependencyStore {
	return &DependencyStore{
		client:      p.Client,
		logger:      p.Logger,
		maxDocCount: p.MaxDocCount,
		rotation:    p.Rotation,
	}
}

// WriteDependencies write dependencies to Elasticsearch
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []dbmodel.DependencyLink) error {
	writeIndexName := s.rotation.WriteTarget(ts)
	s.writeDependenciesToIndex(writeIndexName, ts, dependencies)
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

func (s *DependencyStore) writeDependenciesToIndex(indexName string, ts time.Time, dependencies []dbmodel.DependencyLink) {
	s.client().Index().Index(indexName).Type(dependencyType).
		BodyJson(&dbmodel.TimeDependencies{
			Timestamp:    ts,
			Dependencies: dependencies,
		}).Add()
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]dbmodel.DependencyLink, error) {
	readIndices := s.rotation.ReadTargets(endTs.Add(-lookback), endTs)
	searchResult, err := s.client().Search(readIndices...).
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
		if err := json.Unmarshal(source, &tToD); err != nil {
			return nil, errors.New("unmarshalling ElasticSearch documents failed")
		}
		retDependencies = append(retDependencies, tToD.Dependencies...)
	}
	return retDependencies, nil
}

func buildTSQuery(endTs time.Time, lookback time.Duration) elastic.Query {
	return esquery.NewRangeQuery("timestamp").Gte(endTs.Add(-lookback)).Lte(endTs)
}
