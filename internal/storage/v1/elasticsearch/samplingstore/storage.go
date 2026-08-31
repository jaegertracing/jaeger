// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

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
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore/dbmodel"
)

// IndexExistenceChecker reports whether an index exists. It is the narrow
// admin-plane capability the sampling store needs to find the latest sampling
// index; the factory satisfies it with an esclient.IndicesClient.
type IndexExistenceChecker interface {
	IndexExists(ctx context.Context, index string) (bool, error)
}

type SamplingStore struct {
	searcher    esclient.Searcher
	batchWriter esclient.BatchWriter
	indexClient IndexExistenceChecker
	logger      *zap.Logger
	maxDocCount int
	lookback    time.Duration
	rotation    indices.Rotation
	// now holds the function returning the current time — time.Now by default, but
	// tests replace it with one returning a fixed time to freeze the timestamps
	// written by InsertThroughput and InsertProbabilitiesAndQPS.
	now func() time.Time
}

type Params struct {
	Searcher    esclient.Searcher
	BatchWriter esclient.BatchWriter
	IndexClient IndexExistenceChecker
	Logger      *zap.Logger
	Lookback    time.Duration
	MaxDocCount int
	Rotation    indices.Rotation
}

func NewSamplingStore(p Params) *SamplingStore {
	return &SamplingStore{
		searcher:    p.Searcher,
		batchWriter: p.BatchWriter,
		indexClient: p.IndexClient,
		logger:      p.Logger,
		maxDocCount: p.MaxDocCount,
		lookback:    p.Lookback,
		rotation:    p.Rotation,
		now:         time.Now,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	ts := s.now()
	indexName := s.rotation.WriteTarget(ts)
	converted := dbmodel.FromThroughputs(throughput)
	items := make([]esclient.BulkItem, 0, len(converted))
	for _, eachThroughput := range converted {
		items = append(items, esclient.BulkItem{
			Index: indexName,
			Body: &dbmodel.TimeThroughput{
				Timestamp:  ts,
				Throughput: eachThroughput,
			},
		})
	}
	// context.Background: sampling writes are not request-scoped, and the async
	// indexer this uses ignores the context anyway (see BulkIndexer.WriteBatch).
	return s.batchWriter.WriteBatch(context.Background(), items)
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	readIndices := s.rotation.ReadTargets(start, end)
	searchResult, err := s.searcher.Search(ctx, readIndices, esclient.SearchRequest{
		Size:  s.maxDocCount,
		Query: buildTSQuery(start, end),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for throughputs: %w", err)
	}
	output := make([]dbmodel.TimeThroughput, len(searchResult.Hits.Hits))
	for i, hit := range searchResult.Hits.Hits {
		if err := json.Unmarshal(hit.Source, &output[i]); err != nil {
			return nil, fmt.Errorf("unmarshalling documents failed: %w", err)
		}
	}
	outThroughputs := make([]dbmodel.Throughput, len(output))
	for i, out := range output {
		outThroughputs[i] = out.Throughput
	}
	return dbmodel.ToThroughputs(outThroughputs), nil
}

func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	ts := s.now()
	writeIndexName := s.rotation.WriteTarget(ts)
	val := dbmodel.ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}
	return s.writeProbabilitiesAndQPS(writeIndexName, ts, val)
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ctx := context.Background()
	index, err := s.getLatestIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest index: %w", err)
	}
	searchResult, err := s.searcher.Search(ctx, []string{index}, esclient.SearchRequest{
		Size: s.maxDocCount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for Latest Probabilities: %w", err)
	}
	lengthOfSearchResult := len(searchResult.Hits.Hits)
	if lengthOfSearchResult == 0 {
		return nil, nil
	}

	var latestProbabilities dbmodel.TimeProbabilitiesAndQPS
	latestTime := time.Time{}
	for _, hit := range searchResult.Hits.Hits {
		var data dbmodel.TimeProbabilitiesAndQPS
		if err = json.Unmarshal(hit.Source, &data); err != nil {
			return nil, fmt.Errorf("unmarshalling documents failed: %w", err)
		}
		if data.Timestamp.After(latestTime) {
			latestTime = data.Timestamp
			latestProbabilities = data
		}
	}
	return latestProbabilities.ProbabilitiesAndQPS.Probabilities, nil
}

func (s *SamplingStore) writeProbabilitiesAndQPS(indexName string, ts time.Time, pandqps dbmodel.ProbabilitiesAndQPS) error {
	// context.Background: see InsertThroughput.
	return s.batchWriter.WriteBatch(context.Background(), []esclient.BulkItem{{
		Index: indexName,
		Body: &dbmodel.TimeProbabilitiesAndQPS{
			Timestamp:           ts,
			ProbabilitiesAndQPS: pandqps,
		},
	}})
}

func (s *SamplingStore) getLatestIndex(ctx context.Context) (string, error) {
	now := s.now().UTC()
	candidates := s.rotation.ReadTargets(now.Add(-s.lookback), now)
	for _, idx := range candidates {
		exists, err := s.indexClient.IndexExists(ctx, idx)
		if err != nil {
			return "", fmt.Errorf("failed to check index existence: %w", err)
		}
		if exists {
			return idx, nil
		}
	}
	return "", errors.New("failed to find latest index")
}

func buildTSQuery(start, end time.Time) query.Query {
	return query.NewRangeQuery("timestamp").Gte(start).Lte(end)
}
