// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

import (
	"context"
	"encoding/json"
	"fmt"
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

func NewSamplingStore(client func() es.Client) *SamplingStore {
	return &SamplingStore{
		client: client,
	}
}

// Utility Function to check if the index exists and create it if it doesn't
func (s *SamplingStore) checkIndex() error {
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

// Utility Function to write bulk entries to Elasticsearch
func (s *SamplingStore) writeBulk(entries []elastic.BulkableRequest) error {
	if len(entries) == 0 {
		return nil
	}

	bulkRequest, err := elastic.NewBulkProcessorService(&elastic.Client{}).Workers(5).
		BulkActions(1000).
		FlushInterval(1 * time.Second).
		After(after).
		Do(context.Background())
	for _, entry := range entries {
		bulkRequest.Add(entry)
	}

	return err
}

func after(executionID int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
	if err != nil {
		fmt.Printf("bulk commit failed, err: %v\n", err)
		return
	}
	fmt.Printf("commit successfully, len(requests)=%d\n", len(requests))
}

func (s *SamplingStore) createThroughputEntry(throughput *model.Throughput, startTime time.Time) (*elastic.BulkIndexRequest, error) {
	pK, pV, err := s.createThroughputKV(throughput, startTime)
	if err != nil {
		return nil, err
	}

	req := elastic.NewBulkIndexRequest().
		Index(indexName).
		Type(throughputType).
		Doc(pV).
		Id(string(pK))

	return req, nil
}

func (s *SamplingStore) createThroughputKV(throughput *model.Throughput, startTime time.Time) (string, *model.Throughput, error) {
	key := createElasticsearchKey(startTime)

	return key, throughput, nil
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	err := s.checkIndex()
	if err != nil {
		return err
	}
	var entriesToStore []elastic.BulkableRequest
	for i := range throughput {
		entry, err := s.createThroughputEntry(throughput[i], time.Now())
		if err != nil {
			return err
		}
		entriesToStore = append(entriesToStore, entry)
	}
	return s.writeBulk(entriesToStore)
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ctx := context.Background()
	rangeQuery := elastic.NewRangeQuery("timestamp").
		Gte(start).
		Lte(end)
	esClient := s.client()
	searchQuery := elastic.NewBoolQuery().Filter(rangeQuery)
	result, err := esClient.Search().Query(searchQuery).Do(ctx)
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

func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS) error {
	err := s.checkIndex()
	if err != nil {
		return err
	}

	entry, err := s.createProbabilitiesEntry(hostname, probabilities, qps, time.Now())
	if err != nil {
		return err
	}

	return s.writeBulk([]elastic.BulkableRequest{entry})
}

func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ctx := context.Background()
	esClient := s.client()
	searchQuery := elastic.NewMatchAllQuery()
	result, err := esClient.Search().Query(searchQuery).Size(1).Do(ctx)
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

func (s *SamplingStore) createProbabilitiesEntry(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS, startTime time.Time) (*elastic.BulkIndexRequest, error) {
	pK, pV, err := s.createProbabilitiesKV(hostname, probabilities, qps, startTime)
	if err != nil {
		return nil, err
	}

	req := elastic.NewBulkIndexRequest().
		Index(indexName).
		Type(probabilitiesType).
		Doc(pV).
		Id(string(pK))

	return req, nil
}

func (s *SamplingStore) createProbabilitiesKV(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS, startTime time.Time) (string, *ProbabilitiesAndQPS, error) {
	key := createElasticsearchKey(startTime)

	val := ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}

	return key, &val, nil
}

// Utility function to create an Elasticsearch-compatible key
func createElasticsearchKey(startTime time.Time) string {
	return startTime.Format("2006-01-02T15:04:05")
}
