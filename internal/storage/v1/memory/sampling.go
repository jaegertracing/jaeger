// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"sync"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
)

// SamplingStore is an in-memory store for sampling data
type SamplingStore struct {
	mu                  sync.RWMutex
	throughputs         []*storedThroughput
	probabilitiesAndQPS *storedServiceOperationProbabilitiesAndQPS
	maxBuckets          int
}

type storedThroughput struct {
	throughput []*model.Throughput
	time       time.Time
}

type storedServiceOperationProbabilitiesAndQPS struct {
	hostname      string
	probabilities model.ServiceOperationProbabilities
	qps           model.ServiceOperationQPS
	time          time.Time
}

// NewSamplingStore creates an in-memory sampling store.
func NewSamplingStore(maxBuckets int) *SamplingStore {
	return &SamplingStore{maxBuckets: maxBuckets}
}

// InsertThroughput implements samplingstore.Store#InsertThroughput.
func (ss *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := time.Now()
	ss.preprendThroughput(&storedThroughput{throughput, now})
	return nil
}

// GetThroughput implements samplingstore.Store#GetThroughput.
func (ss *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	var retSlice []*model.Throughput
	for _, t := range ss.throughputs {
		if t.time.After(start) && (t.time.Before(end) || t.time.Equal(end)) {
			retSlice = append(retSlice, t.throughput...)
		}
	}
	return retSlice, nil
}

// InsertProbabilitiesAndQPS implements samplingstore.Store#InsertProbabilitiesAndQPS.
func (ss *SamplingStore) InsertProbabilitiesAndQPS(
	hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.probabilitiesAndQPS = &storedServiceOperationProbabilitiesAndQPS{hostname, probabilities, qps, time.Now()}
	return nil
}

// GetLatestProbabilities implements samplingstore.Store#GetLatestProbabilities.
func (ss *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.probabilitiesAndQPS != nil {
		return ss.probabilitiesAndQPS.probabilities, nil
	}
	return model.ServiceOperationProbabilities{}, nil
}

func (ss *SamplingStore) preprendThroughput(throughput *storedThroughput) {
	ss.throughputs = append([]*storedThroughput{throughput}, ss.throughputs...)
	if len(ss.throughputs) > ss.maxBuckets {
		ss.throughputs = ss.throughputs[0:ss.maxBuckets]
	}
}
