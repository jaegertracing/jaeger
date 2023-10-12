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
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/assert"

	samplemodel "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
)

type samplingStoreTest struct {
	store *SamplingStore
}

func NewtestSamplingStore(db *badger.DB) *samplingStoreTest {
	return &samplingStoreTest{
		store: NewSamplingStore(db),
	}
}

func TestInsertThroughput(t *testing.T) {
	runWithBadger(t, func(s *samplingStoreTest, t *testing.T) {
		throughputs := []*samplemodel.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		err := s.store.InsertThroughput(throughputs)
		assert.NoError(t, err)
	})
}

func TestGetThroughput(t *testing.T) {
	runWithBadger(t, func(s *samplingStoreTest, t *testing.T) {
		start := time.Now()

		expected := 2
		throughputs := []*samplemodel.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		err := s.store.InsertThroughput(throughputs)
		assert.NoError(t, err)

		actual, err := s.store.GetThroughput(start, start.Add(time.Second*time.Duration(10)))
		assert.NoError(t, err)
		assert.Equal(t, expected, len(actual))
	})
}

func runWithBadger(t *testing.T, test func(s *samplingStoreTest, t *testing.T)) {
	opts := badger.DefaultOptions("")

	opts.SyncWrites = false
	dir := t.TempDir()
	opts.Dir = dir
	opts.ValueDir = dir

	store, err := badger.Open(opts)
	defer func() {
		store.Close()
	}()
	ss := NewtestSamplingStore(store)

	assert.NoError(t, err)

	test(ss, t)
}
