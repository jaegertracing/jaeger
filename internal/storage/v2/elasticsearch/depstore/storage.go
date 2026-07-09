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

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
)

// CoreDependencyStore is a DB Level abstraction which directly read/write dependencies into ElasticSearch
type CoreDependencyStore interface {
	// WriteDependencies write dependencies to Elasticsearch
	WriteDependencies(ts time.Time, dependencies []dbmodel.DependencyLink) error
	// GetDependencies returns all interservice dependencies
	GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]dbmodel.DependencyLink, error)
}

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	searcher    esclient.Searcher
	bulkWriter  esclient.BulkWriter
	logger      *zap.Logger
	maxDocCount int
	rotation    indices.Rotation
}

// Params holds constructor parameters for NewDependencyStore
type Params struct {
	Searcher    esclient.Searcher
	BulkWriter  esclient.BulkWriter
	Logger      *zap.Logger
	MaxDocCount int
	Rotation    indices.Rotation
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(p Params) *DependencyStore {
	return &DependencyStore{
		searcher:    p.Searcher,
		bulkWriter:  p.BulkWriter,
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

func (s *DependencyStore) writeDependenciesToIndex(indexName string, ts time.Time, dependencies []dbmodel.DependencyLink) {
	s.bulkWriter.Add(esclient.BulkItem{
		Index: indexName,
		Body: &dbmodel.TimeDependencies{
			Timestamp:    ts,
			Dependencies: dependencies,
		},
	})
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]dbmodel.DependencyLink, error) {
	readIndices := s.rotation.ReadTargets(endTs.Add(-lookback), endTs)
	searchResult, err := s.searcher.Search(ctx, readIndices, esclient.SearchRequest{
		Size:  s.maxDocCount,
		Query: buildTSQuery(endTs, lookback),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for dependencies: %w", err)
	}

	var retDependencies []dbmodel.DependencyLink
	for _, hit := range searchResult.Hits.Hits {
		var tToD dbmodel.TimeDependencies
		if err := json.Unmarshal(hit.Source, &tToD); err != nil {
			return nil, errors.New("unmarshalling ElasticSearch documents failed")
		}
		retDependencies = append(retDependencies, tToD.Dependencies...)
	}
	return retDependencies, nil
}

func buildTSQuery(endTs time.Time, lookback time.Duration) query.Query {
	return query.NewRangeQuery("timestamp").Gte(endTs.Add(-lookback)).Lte(endTs)
}
