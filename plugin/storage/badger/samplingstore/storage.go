// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger/v4"
	jaegermodel "github.com/jaegertracing/jaeger-idl/model/v1"

	"github.com/jaegertracing/jaeger/storage/samplingstore/model"
)

const (
	throughputKeyPrefix    byte = 0x08
	probabilitiesKeyPrefix byte = 0x09
)

type SamplingStore struct {
	store *badger.DB
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities model.ServiceOperationProbabilities
	QPS           model.ServiceOperationQPS
}

func NewSamplingStore(db *badger.DB) *SamplingStore {
	return &SamplingStore{
		store: db,
	}
}

func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	startTime := jaegermodel.TimeAsEpochMicroseconds(time.Now())
	entriesToStore := make([]*badger.Entry, 0)
	entries, err := s.createThroughputEntry(throughput, startTime)
	if err != nil {
		return err
	}
	entriesToStore = append(entriesToStore, entries)
	err = s.store.Update(func(txn *badger.Txn) error {
		for i := range entriesToStore {
			err = txn.SetEntry(entriesToStore[i])
			if err != nil {
				return err
			}
		}

		return nil
	})

	return nil
}

func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	var retSlice []*model.Throughput
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
			val, err := item.ValueCopy(val)
			if err != nil {
				return err
			}
			t, err := initalStartTime(startTime)
			if err != nil {
				return err
			}
			throughputs, err := decodeThroughputValue(val)
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
	startTime := jaegermodel.TimeAsEpochMicroseconds(time.Now())
	entriesToStore := make([]*badger.Entry, 0)
	entries, err := s.createProbabilitiesEntry(hostname, probabilities, qps, startTime)
	if err != nil {
		return err
	}
	entriesToStore = append(entriesToStore, entries)
	err = s.store.Update(func(txn *badger.Txn) error {
		// Write the entries
		for i := range entriesToStore {
			err = txn.SetEntry(entriesToStore[i])
			if err != nil {
				return err
			}
		}

		return nil
	})

	return nil
}

// GetLatestProbabilities implements samplingstore.Reader#GetLatestProbabilities.
func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	var retVal model.ServiceOperationProbabilities
	var unMarshalProbabilities ProbabilitiesAndQPS
	prefix := []byte{probabilitiesKeyPrefix}

	err := s.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		val := []byte{}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			val, err := item.ValueCopy(val)
			if err != nil {
				return err
			}
			unMarshalProbabilities, err = decodeProbabilitiesValue(val)
			if err != nil {
				return err
			}
			retVal = unMarshalProbabilities.Probabilities
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return retVal, nil
}

func (s *SamplingStore) createProbabilitiesEntry(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS, startTime uint64) (*badger.Entry, error) {
	pK, pV, err := s.createProbabilitiesKV(hostname, probabilities, qps, startTime)
	if err != nil {
		return nil, err
	}

	e := s.createBadgerEntry(pK, pV)

	return e, nil
}

func (*SamplingStore) createProbabilitiesKV(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS, startTime uint64) ([]byte, []byte, error) {
	key := make([]byte, 16)
	key[0] = probabilitiesKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], startTime)

	var bb []byte
	var err error
	val := ProbabilitiesAndQPS{
		Hostname:      hostname,
		Probabilities: probabilities,
		QPS:           qps,
	}
	bb, err = json.Marshal(val)
	return key, bb, err
}

func (s *SamplingStore) createThroughputEntry(throughput []*model.Throughput, startTime uint64) (*badger.Entry, error) {
	pK, pV, err := s.createThroughputKV(throughput, startTime)
	if err != nil {
		return nil, err
	}

	e := s.createBadgerEntry(pK, pV)

	return e, nil
}

func (*SamplingStore) createBadgerEntry(key []byte, value []byte) *badger.Entry {
	return &badger.Entry{
		Key:   key,
		Value: value,
	}
}

func (*SamplingStore) createThroughputKV(throughput []*model.Throughput, startTime uint64) ([]byte, []byte, error) {
	key := make([]byte, 16)
	key[0] = throughputKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], startTime)

	var bb []byte
	var err error

	bb, err = json.Marshal(throughput)
	return key, bb, err
}

func decodeThroughputValue(val []byte) ([]*model.Throughput, error) {
	var throughput []*model.Throughput

	err := json.Unmarshal(val, &throughput)
	if err != nil {
		return nil, err
	}
	return throughput, err
}

func decodeProbabilitiesValue(val []byte) (ProbabilitiesAndQPS, error) {
	var probabilities ProbabilitiesAndQPS

	err := json.Unmarshal(val, &probabilities)
	if err != nil {
		return ProbabilitiesAndQPS{}, err
	}
	return probabilities, nil
}

func initalStartTime(timeBytes []byte) (time.Time, error) {
	var usec int64

	buf := bytes.NewReader(timeBytes)

	if err := binary.Read(buf, binary.BigEndian, &usec); err != nil {
		return time.Time{}, err
	}

	t := time.UnixMicro(usec)
	return t, nil
}
