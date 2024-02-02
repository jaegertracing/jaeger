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
	"time"

	"github.com/olivere/elastic"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/es"
)

const (
	indexName         = "sampling"
	throughputType    = "throughput"
	probabilitiesType = "probabilities"
)

type SamplingStore struct {
	client func() es.Client
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities model.ServiceOperationProbabilities
	QPS           model.ServiceOperationQPS
}

func NewSamplingStore(clientFn func() es.Client) *SamplingStore {
	return &SamplingStore{
		client: clientFn,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	err := s.createIndexIfNotExists()
	if err != nil {
		return err
	}
	for i := range throughput {
		s.write(throughputType, throughput[i])
	}
	return nil
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	client := s.client()
	rangeQuery := elastic.NewRangeQuery("timestamp").
		Gte(start).
		Lte(end)

	searchQuery := elastic.NewBoolQuery().Filter(rangeQuery)
	result, err := client.Search().Query(searchQuery).Do(ctx)
	if err != nil {
		return nil, err
	}

	var throughputs []*model.Throughput
	for _, hit := range result.Hits.Hits {
		var throughput model.Throughput
		err := json.Unmarshal(*hit.Source, &throughput)
		if err != nil {
			return nil, err
		}
		throughputs = append(throughputs, &throughput)
	}

	return throughputs, nil
}

func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	err := s.createIndexIfNotExists()
	if err != nil {
		return err
	}

	val := ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}
	s.write(probabilitiesType, val)
	return err
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ctx := context.Background()
	client := s.client()
	query := elastic.NewMatchAllQuery()
	result, err := client.Search().Query(query).Size(1).Do(ctx)
	if err != nil {
		return nil, err
	}

	if len(result.Hits.Hits) == 0 {
		return nil, nil
	}

	hit := result.Hits.Hits[0]
	var unMarshalProbabilities ProbabilitiesAndQPS
	err = json.Unmarshal(*hit.Source, &unMarshalProbabilities)
	if err != nil {
		return nil, err
	}

	return unMarshalProbabilities.Probabilities, nil
}

func (s *SamplingStore) createIndexIfNotExists() error {
	ctx := context.Background()
	exists, err := s.client().IndexExists(indexName).Do(ctx)
	if err != nil {
		return err
	}
	if !exists {
		_, err := s.client().CreateIndex(indexName).Do(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SamplingStore) write(typeEs string, jsonSpan interface{}) {
	s.client().Index().Index(indexName).Type(typeEs).BodyJson(&jsonSpan).Add()
}
