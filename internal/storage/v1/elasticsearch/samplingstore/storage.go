// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

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
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore/dbmodel"
)

const (
	samplingIndexBaseName = "jaeger-sampling"
	throughputType        = "throughput-sampling"
	probabilitiesType     = "probabilities-sampling"
)

type SamplingStore struct {
	client      func() es.Client
	logger      *zap.Logger
	maxDocCount int
	lookback    time.Duration
	rotation    indices.Rotation
}

type Params struct {
	Client      func() es.Client
	Logger      *zap.Logger
	Lookback    time.Duration
	MaxDocCount int
	Rotation    indices.Rotation
}

func NewSamplingStore(p Params) *SamplingStore {
	return &SamplingStore{
		client:      p.Client,
		logger:      p.Logger,
		maxDocCount: p.MaxDocCount,
		lookback:    p.Lookback,
		rotation:    p.Rotation,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	ts := time.Now()
	indexName := s.rotation.WriteTarget(ts)
	for _, eachThroughput := range dbmodel.FromThroughputs(throughput) {
		s.client().Index().Index(indexName).Type(throughputType).
			BodyJson(&dbmodel.TimeThroughput{
				Timestamp:  ts,
				Throughput: eachThroughput,
			}).Add()
	}
	return nil
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	readIndices := s.rotation.ReadTargets(start, end)
	searchResult, err := s.client().Search(readIndices...).
		Size(s.maxDocCount).
		Query(buildTSQuery(start, end)).
		IgnoreUnavailable(true).
		Do(ctx)
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
	ts := time.Now()
	writeIndexName := s.rotation.WriteTarget(ts)
	val := dbmodel.ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}
	s.writeProbabilitiesAndQPS(writeIndexName, ts, val)
	return nil
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ctx := context.Background()
	clientFn := s.client()
	index, err := s.getLatestIndex(ctx, clientFn)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest indices: %w", err)
	}
	searchResult, err := clientFn.Search(index).
		Size(s.maxDocCount).
		IgnoreUnavailable(true).
		Do(ctx)
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

func (s *SamplingStore) writeProbabilitiesAndQPS(indexName string, ts time.Time, pandqps dbmodel.ProbabilitiesAndQPS) {
	s.client().Index().Index(indexName).Type(probabilitiesType).
		BodyJson(&dbmodel.TimeProbabilitiesAndQPS{
			Timestamp:           ts,
			ProbabilitiesAndQPS: pandqps,
		}).Add()
}

func (s *SamplingStore) getLatestIndex(ctx context.Context, client es.Client) (string, error) {
	candidates := s.rotation.ReadTargets(time.Now().UTC().Add(-s.lookback), time.Now().UTC())
	for _, idx := range candidates {
		exists, err := client.IndexExists(idx).Do(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to check index existence: %w", err)
		}
		if exists {
			return idx, nil
		}
	}
	return "", errors.New("failed to find latest index")
}

func buildTSQuery(start, end time.Time) elastic.Query {
	return esquery.NewRangeQuery("timestamp").Gte(start).Lte(end)
}
