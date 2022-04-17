// Copyright (c) 2021 The Jaeger Authors.
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

package memory

import (
	"sync"
	"time"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
)

// SamplingStore is an in-memory store for sampling data
type SamplingStore struct {
	sync.RWMutex
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
	ss.Lock()
	defer ss.Unlock()
	now := time.Now()
	ss.preprendThroughput(&storedThroughput{throughput, now})
	return nil
}

// GetThroughput implements samplingstore.Store#GetThroughput.
func (ss *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	ss.Lock()
	defer ss.Unlock()
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
	ss.Lock()
	defer ss.Unlock()
	ss.probabilitiesAndQPS = &storedServiceOperationProbabilitiesAndQPS{hostname, probabilities, qps, time.Now()}
	return nil
}

// GetLatestProbabilities implements samplingstore.Store#GetLatestProbabilities.
func (ss *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ss.Lock()
	defer ss.Unlock()
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
