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
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/olivere/elastic"
)

const (
	samplingType = "sampling"
)

type SamplingStore struct {
	client func() es.Client
	// logger           *zap.Logger
	// writerMetrics    spanWriterMetrics // TODO: build functions to wrap around each Do fn
	// indexCache       cache.Cache
	// serviceWriter    serviceWriter
	// spanConverter    dbmodel.FromDomain
	// spanServiceIndex spanAndServiceIndexFn
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities model.ServiceOperationProbabilities
	QPS           model.ServiceOperationQPS
}

// type SamplingStore struct {
// 	store *badger.DB
// }

func NewSamplingStore(db *badger.DB) *atomic.Pointer[es.Client] {
	return &atomic.Pointer[es.Client]{
		// store: db,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	for i := range throughput {
		// spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)
		// // converts model.Span into json.Span
		// if serviceIndexName != "" {
		// 	s.writeService(serviceIndexName, throughput[i])
		// }
		// s.writeSpan(spanIndexName, throughput[i])
		s.writeSpan(throughput[i])
	}
	return nil
}

func (s *SamplingStore) writeSpan(jsonSpan *model.Throughput) {
	s.client().Index().Type(samplingType).BodyJson(&jsonSpan).Add()
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	rangeQuery := elastic.NewRangeQuery("timestamp").
		Gte(start).
		Lte(end)

	boolQuery := elastic.NewBoolQuery().Filter(rangeQuery)
	result, err := s.client().Search().Query(boolQuery).Do(ctx)
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
	val := ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}
	jsonSpan, err := json.Marshal(val)
	if err != nil {
		return err
	}
	s.client().Index().Type(samplingType).BodyJson(&jsonSpan).Add()
	return nil
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	var retVal model.ServiceOperationProbabilities
	var unMarshalProbabilities ProbabilitiesAndQPS
	ctx := context.Background()
	result, err := s.client().Search().Do(ctx) //fix
	if err != nil {
		return nil, err
	}
	val := []byte{}
	for _, hit := range result.Hits.Hits {
		err := json.Unmarshal(*hit.Source, &val)
		if err != nil {
			return nil, err
		}
		unMarshalProbabilities, err = decodeProbabilitiesValue(val)
		if err != nil {
			return nil, err
		}
		retVal = unMarshalProbabilities.Probabilities
	}
	return retVal, nil
}

func decodeProbabilitiesValue(val []byte) (ProbabilitiesAndQPS, error) {
	var probabilities ProbabilitiesAndQPS

	err := json.Unmarshal(val, &probabilities)
	if err != nil {
		return ProbabilitiesAndQPS{}, err
	}
	return probabilities, nil
}
