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
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	// jaegermodel "github.com/jaegertracing/jaeger/model"
)

const (
	samplingType = "sampling"
)

// create all this
// 	// InsertThroughput inserts aggregated throughput for operations into storage.
// 	InsertThroughput(throughput []*model.Throughput) error

// 	// InsertProbabilitiesAndQPS inserts calculated sampling probabilities and measured qps into storage.
// 	InsertProbabilitiesAndQPS(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS) error

// 	// GetThroughput retrieves aggregated throughput for operations within a time range.
// 	GetThroughput(start, end time.Time) ([]*model.Throughput, error)

// 	// GetLatestProbabilities retrieves the latest sampling probabilities.
// 	GetLatestProbabilities() (model.ServiceOperationProbabilities, error)

type SamplingStore struct {
	client func() es.Client
	// logger           *zap.Logger
	// writerMetrics    spanWriterMetrics // TODO: build functions to wrap around each Do fn
	// indexCache       cache.Cache
	// serviceWriter    serviceWriter
	// spanConverter    dbmodel.FromDomain
	// spanServiceIndex spanAndServiceIndexFn
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
	// startTime := jaegermodel.TimeAsEpochMicroseconds(time.Now())
	// entriesToStore := make([]*badger.Entry, 0)
	// // here converting *model.Throughput to *badger.Entry
	// entries, err := s.createThroughputEntry(throughput, startTime)
	// if err != nil {
	// 	return err
	// }
	// entriesToStore = append(entriesToStore, entries)
	// err = s.store.Update(func(txn *badger.Txn) error {
	// 	for i := range entriesToStore {
	// 		err = txn.SetEntry(entriesToStore[i])
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}

	// 	return nil
	// })

	// return nil
	for i := range throughput {
		// spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)
		// // converts model.Span into json.Span
		// if serviceIndexName != "" {
		// 	s.writeService(serviceIndexName, throughput[i])
		// }
		// s.writeSpan(spanIndexName, throughput[i])
		s.writeSpan(throughput[i])
		return nil
	}
	return nil
}

// func (s *SamplingStore) writeSpan(indexName string, jsonSpan *model.Throughput) {
func (s *SamplingStore) writeSpan(jsonSpan *model.Throughput) {
	// s.client().Index().Index(indexName).Type(samplingType).BodyJson(&jsonSpan).Add()
	s.client().Index().Type(samplingType).BodyJson(&jsonSpan).Add()
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	// 	var retSlice []*model.Throughput
	// 	prefix := []byte{throughputKeyPrefix}

	// 	err := s.store.View(func(txn *badger.Txn) error {
	// 		opts := badger.DefaultIteratorOptions
	// 		it := txn.NewIterator(opts)
	// 		defer it.Close()

	// 		val := []byte{}
	// 		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
	// 			item := it.Item()
	// 			k := item.Key()
	// 			startTime := k[1:9]
	// 			val, err := item.ValueCopy(val)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			t, err := initalStartTime(startTime)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			throughputs, err := decodeThroughputValue(val)
	// 			if err != nil {
	// 				return err
	// 			}

	// 			if t.After(start) && (t.Before(end) || t.Equal(end)) {
	// 				retSlice = append(retSlice, throughputs...)
	// 			}
	// 		}
	// 		return nil
	// 	})
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// return retSlice, nil
}

// func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string,
// 	probabilities model.ServiceOperationProbabilities,
// 	qps model.ServiceOperationQPS,
// ) error {
// 	startTime := jaegermodel.TimeAsEpochMicroseconds(time.Now())
// 	entriesToStore := make([]*badger.Entry, 0)
// 	entries, err := s.createProbabilitiesEntry(hostname, probabilities, qps, startTime)
// 	if err != nil {
// 		return err
// 	}
// 	entriesToStore = append(entriesToStore, entries)
// 	err = s.store.Update(func(txn *badger.Txn) error {
// 		// Write the entries
// 		for i := range entriesToStore {
// 			err = txn.SetEntry(entriesToStore[i])
// 			if err != nil {
// 				return err
// 			}
// 		}

// 		return nil
// 	})

// 	return nil
// }

// func (ss *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
// 	ss.Lock()
// 	defer ss.Unlock()
// 	if ss.probabilitiesAndQPS != nil {
// 		return ss.probabilitiesAndQPS.probabilities, nil
// 	}
// 	return model.ServiceOperationProbabilities{}, nil
// }
