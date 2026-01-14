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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
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
	client                 func() es.Client
	logger                 *zap.Logger
	samplingIndexPrefix    string
	indexDateLayout        string
	maxDocCount            int
	indexRolloverFrequency time.Duration
	lookback               time.Duration
}

type Params struct {
	Client                 func() es.Client
	Logger                 *zap.Logger
	IndexPrefix            config.IndexPrefix
	IndexDateLayout        string
	IndexRolloverFrequency time.Duration
	Lookback               time.Duration
	MaxDocCount            int
}

func NewSamplingStore(p Params) *SamplingStore {
	return &SamplingStore{
		client:                 p.Client,
		logger:                 p.Logger,
		samplingIndexPrefix:    p.IndexPrefix.Apply(samplingIndexBaseName) + config.IndexPrefixSeparator,
		indexDateLayout:        p.IndexDateLayout,
		maxDocCount:            p.MaxDocCount,
		indexRolloverFrequency: p.IndexRolloverFrequency,
		lookback:               p.Lookback,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	ts := time.Now()
	indexName := s.getWriteIndex(ts)
	for _, eachThroughput := range dbmodel.FromThroughputs(throughput) {
		s.client().Index().Index(indexName).Type(throughputType).
			BodyJson(&dbmodel.TimeThroughput{
				Timestamp:  ts,
				Throughput: eachThroughput,
			}).Add("")
	}
	return nil
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	indices := s.getReadIndices(start, end)
	searchResult, err := s.client().Search(indices...).
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

func (s *SamplingStore) InsertProbabilitiesAndQPS(_ string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	ts := time.Now()
	writeIndexName := s.getWriteIndex(ts)
	val := dbmodel.ProbabilitiesAndQPS{
		Probabilities: probabilities,
		QPS:           qps,
	}
	s.writeProbabilitiesAndQPS(writeIndexName, ts, val)
	return nil
}

func (s *SamplingStore) getWriteIndex(ts time.Time) string {
	return config.IndexWithDate(s.samplingIndexPrefix, s.indexDateLayout, ts)
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ctx := context.Background()
	clientFn := s.client()
	indices, err := s.getLatestIndices()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest indices: %w", err)
	}
	searchResult, err := clientFn.Search(indices...).
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
		}).Add("")
}

func (s *SamplingStore) getLatestIndices() ([]string, error) {
	clientFn := s.client()
	ctx := context.Background()
	now := time.Now().UTC()
	earliest := now.Add(-s.lookback)
	earliestIndex := config.IndexWithDate(s.samplingIndexPrefix, s.indexDateLayout, earliest)
	for {
		currentIndex := config.IndexWithDate(s.samplingIndexPrefix, s.indexDateLayout, now)
		exists, err := clientFn.IndexExists(currentIndex).Do(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check index existence: %w", err)
		}
		if exists {
			return []string{currentIndex}, nil
		}
		if currentIndex == earliestIndex {
			return nil, errors.New("falied to find latest index")
		}
		now = now.Add(s.indexRolloverFrequency) // rollover is negative
	}
}

func (s *SamplingStore) getReadIndices(startTime time.Time, endTime time.Time) []string {
	var indices []string
	firstIndex := config.IndexWithDate(s.samplingIndexPrefix, s.indexDateLayout, startTime)
	currentIndex := config.IndexWithDate(s.samplingIndexPrefix, s.indexDateLayout, endTime)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		endTime = endTime.Add(s.indexRolloverFrequency) // rollover is negative
		currentIndex = config.IndexWithDate(s.samplingIndexPrefix, s.indexDateLayout, endTime)
	}
	indices = append(indices, firstIndex)
	return indices
}

func (p *Params) PrefixedIndexName() string {
	return p.IndexPrefix.Apply(samplingIndexBaseName)
}

func buildTSQuery(start, end time.Time) elastic.Query {
	return esquery.NewRangeQuery("timestamp").Gte(start).Lte(end)
}
