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
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/olivere/elastic"
)

const (
	samplingIndex        = "jaeger-sampling-"
	throughputType       = "throughput-sampling"
	probabilitiesType    = "probabilities-sampling"
	indexPrefixSeparator = "-"
)

type SamplingStore struct {
	client              func() es.Client
	logger              *zap.Logger
	samplingIndexPrefix string
	indexDateLayout     string
	maxDocCount         int
}

type SamplingStoreParams struct {
	Client          func() es.Client
	Logger          *zap.Logger
	IndexPrefix     string
	IndexDateLayout string
	MaxDocCount     int
}

type TimeThroughput struct {
	Timestamp  time.Time           `json:"timestamp"`
	Throughput []*model.Throughput `json:"dependencies"`
}

type TimeProbabilitiesAndQPS struct {
	Timestamp  time.Time           `json:"timestamp"`
	Throughput ProbabilitiesAndQPS `json:"dependencies"`
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities model.ServiceOperationProbabilities
	QPS           model.ServiceOperationQPS
}

func NewSamplingStore(p SamplingStoreParams) *SamplingStore {
	return &SamplingStore{
		client:              p.Client,
		logger:              p.Logger,
		samplingIndexPrefix: prefixIndexName(p.IndexPrefix, samplingIndex),
		indexDateLayout:     p.IndexDateLayout,
		maxDocCount:         p.MaxDocCount,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	ts := time.Now()
	writeIndexName := indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, ts)
	s.writeThroughput(writeIndexName, ts, throughput)
	return nil
}

func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	ts := time.Now()
	writeIndexName := indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, ts)
	val := ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}
	s.writeProbabilitiesAndQPS(writeIndexName, ts, val)
	return nil
}

func (s *SamplingStore) writeProbabilitiesAndQPS(indexName string, ts time.Time, dependencies ProbabilitiesAndQPS) {
	s.client().Index().Index(indexName).Type(probabilitiesType).
		BodyJson(&TimeProbabilitiesAndQPS{
			Timestamp:  ts,
			Throughput: dependencies,
		}).Add()
}

func indexWithDate(indexNamePrefix, indexDateLayout string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format(indexDateLayout)
}

func (s *SamplingStore) writeThroughput(indexName string, ts time.Time, dependencies []*model.Throughput) {
	s.client().Index().Index(indexName).Type(throughputType).
		BodyJson(&TimeThroughput{
			Timestamp:  ts,
			Throughput: dependencies,
		}).Add()
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
	var retSamples []*model.Throughput
	hits := searchResult.Hits.Hits
	for _, hit := range hits {
		source := hit.Source
		var tToD TimeThroughput
		if err := json.Unmarshal(*source, &tToD); err != nil {
			return nil, errors.New("unmarshalling ElasticSearch documents failed12312")
		}
		retSamples = append(retSamples, tToD.Throughput...)
	}
	return retSamples, nil
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ctx := context.Background()
	searchResult, err := s.client().Search().
		Size(1).
		IgnoreUnavailable(true).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to search for Latest Probabilities: %w", err)
	}
	if len(searchResult.Hits.Hits) == 0 {
		return nil, nil
	}
	hit := searchResult.Hits.Hits[0]
	var unMarshalProbabilities ProbabilitiesAndQPS
	err = json.Unmarshal(*hit.Source, &unMarshalProbabilities)
	if err != nil {
		return nil, err
	}

	return unMarshalProbabilities.Probabilities, nil
}

func buildTSQuery(start, end time.Time) elastic.Query {
	return elastic.NewRangeQuery("timestamp").Gte(start).Lte(end)
}

func (s *SamplingStore) getReadIndices(start, end time.Time) []string {
	var indices []string
	firstIndex := indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, start)
	currentIndex := indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, end)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		end = end.Add(-24 * time.Hour)
		currentIndex = indexWithDate(s.samplingIndexPrefix, s.indexDateLayout, end)
	}
	return append(indices, firstIndex)
}

func prefixIndexName(prefix, index string) string {
	if prefix != "" {
		return prefix + indexPrefixSeparator + index
	}
	return index
}
