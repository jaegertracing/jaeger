// Copyright (c) 2023 The Jaeger Authors.
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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v3"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	jaegermodel "github.com/jaegertracing/jaeger/model"
)

const (
	throughputKeyPrefix    byte = 0x08
	probabilitiesKeyPrefix byte = 0x09
)

type SamplingStore struct {
	store *badger.DB
}

func NewSamplingStore(db *badger.DB) *SamplingStore {
	return &SamplingStore{
		store: db,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	fmt.Println("Inside badger samplingstore InsertThroughput")
	startTime := jaegermodel.TimeAsEpochMicroseconds(time.Now())
	entriesToStore := make([]*badger.Entry, 0)
	entries, err := s.createThroughputEntry(throughput, startTime)
	if err != nil {
		return err
	}
	entriesToStore = append(entriesToStore, entries)
	err = s.store.Update(func(txn *badger.Txn) error {
		// Write the entries
		for i := range entriesToStore {
			err = txn.SetEntry(entriesToStore[i])
			fmt.Println("Writing entry to badger")
			if err != nil {
				// Most likely primary key conflict, but let the caller check this
				return err
			}
		}

		return nil
	})

	return nil
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	var retSlice []*model.Throughput
	fmt.Println("Inside badger samplingstore GetThroughput")

	prefix := []byte{throughputKeyPrefix}

	err := s.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		val := []byte{}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			startTime := k[1:9]
			fmt.Printf("key=%s\n", k)
			val, err := item.ValueCopy(val)
			if err != nil {
				return err
			}
			t, err := initalStartTime(startTime)
			if err != nil {
				return err
			}
			throughputs, err := decodeValue(val)
			if err != nil {
				return err
			}

			if t.After(start) && (t.Before(end) || t.Equal(end)) {
				retSlice = append(retSlice, throughputs...)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return retSlice, nil
}

func (s *SamplingStore) InsertProbabilitiesAndQPS(hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	return nil
}

// GetLatestProbabilities implements samplingstore.Reader#GetLatestProbabilities.
func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	return nil, nil
}

func (s *SamplingStore) createThroughputEntry(throughput []*model.Throughput, startTime uint64) (*badger.Entry, error) {
	pK, pV, err := s.createThroughputKV(throughput, startTime)
	if err != nil {
		return nil, err
	}

	e := s.createBadgerEntry(pK, pV)

	return e, nil
}

func (s *SamplingStore) createBadgerEntry(key []byte, value []byte) *badger.Entry {
	return &badger.Entry{
		Key:   key,
		Value: value,
	}
}

func (s *SamplingStore) createThroughputKV(throughput []*model.Throughput, startTime uint64) ([]byte, []byte, error) {
	key := make([]byte, 16)
	key[0] = throughputKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], startTime)

	var bb []byte
	var err error

	bb, err = json.Marshal(throughput)
	fmt.Printf("Badger key %v, value %v\n", key, string(bb))
	return key, bb, err
}

func decodeValue(val []byte) ([]*model.Throughput, error) {
	var throughput []*model.Throughput

	err := json.Unmarshal(val, &throughput)
	if err != nil {
		fmt.Println("Error while unmarshalling")
		return nil, err
	}
	fmt.Printf("Throughput %v\n", throughput)
	return throughput, nil
}

func initalStartTime(timeBytes []byte) (time.Time, error) {
	var usec int64

	buf := bytes.NewReader(timeBytes)

	if err := binary.Read(buf, binary.BigEndian, &usec); err != nil {
		panic(nil)
	}

	t := time.UnixMicro(usec)
	return t, nil
}
