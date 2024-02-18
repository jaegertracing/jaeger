// Copyright (c) 2024 The Jaeger Authors.
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

package samplingstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/olivere/elastic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/samplingstore/dbmodel"
)

const (
	samplingIndex        = "jaeger-sampling-"
	throughputType       = "throughput-sampling"
	probabilitiesType    = "probabilities-sampling"
	indexPrefixSeparator = "-"
)

type SamplingStore struct {
	client                         func() es.Client
	logger                         *zap.Logger
	samplingIndexPrefix            string
	indexDateLayout                string
	maxDocCount                    int
	samplingIndexRolloverFrequency time.Duration
	adaptiveSamplingLookback       time.Duration
}

type SamplingStoreParams struct {
	Client                         func() es.Client
	Logger                         *zap.Logger
	IndexPrefix                    string
	IndexDateLayout                string
	MaxDocCount                    int
	SamplingIndexRolloverFrequency time.Duration
	AdaptiveSamplingLookback       time.Duration
}

func NewSamplingStore(p SamplingStoreParams) *SamplingStore {
	return &SamplingStore{
		client:                         p.Client,
		logger:                         p.Logger,
		samplingIndexPrefix:            p.prefixIndexName(),
		indexDateLayout:                p.IndexDateLayout,
		maxDocCount:                    p.MaxDocCount,
		samplingIndexRolloverFrequency: p.SamplingIndexRolloverFrequency,
		adaptiveSamplingLookback:       p.AdaptiveSamplingLookback,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	ts := time.Now()
	writeIndexName := indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, ts)
	s.writeThroughput(writeIndexName, ts, throughput)
	return nil
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	indices := getReadIndices(s.samplingIndexPrefix, s.indexDateLayout, start, end, s.samplingIndexRolloverFrequency)
	searchResult, err := s.client().Search(indices...).
		Size(s.maxDocCount).
		Query(buildTSQuery(start, end)).
		IgnoreUnavailable(true).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to search for throughputs: %w", err)
	}
	var retSamples []dbmodel.Throughput
	hits := searchResult.Hits.Hits
	for _, hit := range hits {
		source := hit.Source
		var tToD dbmodel.TimeThroughput
		if err := json.Unmarshal(*source, &tToD); err != nil {
			return nil, fmt.Errorf("unmarshalling documents failed: %w", err)
		}
		retSamples = append(retSamples, tToD.Throughput...)
	}
	return dbmodel.ToThroughputs(retSamples), nil
}

func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	ts := time.Now()
	writeIndexName := indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, ts)
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
	indices, err := getLatestIndices(s.samplingIndexPrefix, s.indexDateLayout, clientFn, s.samplingIndexRolloverFrequency, s.adaptiveSamplingLookback)
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
		if err = json.Unmarshal(*hit.Source, &data); err != nil {
			return nil, fmt.Errorf("unmarshalling documents failed: %w", err)
		}
		if data.Timestamp.After(latestTime) {
			latestTime = data.Timestamp
			latestProbabilities = data
		}
	}
	return latestProbabilities.ProbabilitiesAndQPS.Probabilities, nil
}

func (s *SamplingStore) writeThroughput(indexName string, ts time.Time, throughputs []*model.Throughput) {
	s.client().Index().Index(indexName).Type(throughputType).
		BodyJson(&dbmodel.TimeThroughput{
			Timestamp:  ts,
			Throughput: dbmodel.FromThroughputs(throughputs),
		}).Add()
}

func (s *SamplingStore) writeProbabilitiesAndQPS(indexName string, ts time.Time, pandqps dbmodel.ProbabilitiesAndQPS) {
	s.client().Index().Index(indexName).Type(probabilitiesType).
		BodyJson(&dbmodel.TimeProbabilitiesAndQPS{
			Timestamp:           ts,
			ProbabilitiesAndQPS: pandqps,
		}).Add()
}

func getLatestIndices(indexPrefix, indexDateLayout string, clientFn es.Client, rollover time.Duration, maxDuration time.Duration) ([]string, error) {
	ctx := context.Background()
	now := time.Now().UTC()
	end := now.Add(-maxDuration)
	for {
		indexName := indexWithDate(indexPrefix, indexDateLayout, now)
		exists, err := clientFn.IndexExists(indexName).Do(ctx)
		if err != nil {
			return nil, err
		}
		if exists {
			return []string{indexName}, nil
		}
		if now == end {
			return nil, fmt.Errorf("falied to find latest index")
		}
		now = now.Add(rollover)
	}
}

func getReadIndices(indexName, indexDateLayout string, startTime time.Time, endTime time.Time, rollover time.Duration) []string {
	lastIndex := indexWithDate(indexName, indexDateLayout, endTime)
	indices := []string{lastIndex}
	currentIndex := indexWithDate(indexName, indexDateLayout, startTime)
	for currentIndex != lastIndex {
		indices = append(indices, currentIndex)
		startTime = startTime.Add(-rollover)
		currentIndex = indexWithDate(indexName, indexDateLayout, startTime)
	}
	return indices
}

func (p *SamplingStoreParams) prefixIndexName() string {
	if p.IndexPrefix != "" {
		return p.IndexPrefix + indexPrefixSeparator + samplingIndex
	}
	return samplingIndex
}

func buildTSQuery(start, end time.Time) elastic.Query {
	return elastic.NewRangeQuery("timestamp").Gte(start).Lte(end)
}

func indexWithDate(indexNamePrefix, indexDateLayout string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format(indexDateLayout)
}
