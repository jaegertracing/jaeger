// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	samplemodel "github.com/jaegertracing/jaeger/storage/samplingstore/model"
)

func newTestSamplingStore(db *badger.DB) *SamplingStore {
	return NewSamplingStore(db)
}

func TestInsertThroughput(t *testing.T) {
	runWithBadger(t, func(t *testing.T, store *SamplingStore) {
		throughputs := []*samplemodel.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		err := store.InsertThroughput(throughputs)
		require.NoError(t, err)
	})
}

func TestGetThroughput(t *testing.T) {
	runWithBadger(t, func(t *testing.T, store *SamplingStore) {
		start := time.Now()
		expected := []*samplemodel.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		err := store.InsertThroughput(expected)
		require.NoError(t, err)

		actual, err := store.GetThroughput(start.Add(-time.Millisecond), start.Add(time.Second*time.Duration(10)))
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func TestInsertProbabilitiesAndQPS(t *testing.T) {
	runWithBadger(t, func(t *testing.T, store *SamplingStore) {
		err := store.InsertProbabilitiesAndQPS(
			"dell11eg843d",
			samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
			samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
		)
		require.NoError(t, err)
	})
}

func TestGetLatestProbabilities(t *testing.T) {
	runWithBadger(t, func(t *testing.T, store *SamplingStore) {
		err := store.InsertProbabilitiesAndQPS(
			"dell11eg843d",
			samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
			samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
		)
		require.NoError(t, err)
		err = store.InsertProbabilitiesAndQPS(
			"newhostname",
			samplemodel.ServiceOperationProbabilities{"new-srv2": {"op": 0.123}},
			samplemodel.ServiceOperationQPS{"new-srv2": {"op": 1}},
		)
		require.NoError(t, err)

		expected := samplemodel.ServiceOperationProbabilities{"new-srv2": {"op": 0.123}}
		actual, err := store.GetLatestProbabilities()
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func TestDecodeProbabilitiesValue(t *testing.T) {
	expected := ProbabilitiesAndQPS{
		Hostname:      "dell11eg843d",
		Probabilities: samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
		QPS:           samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
	}

	marshalBytes, err := json.Marshal(expected)
	require.NoError(t, err)
	// This should pass without error
	actual, err := decodeProbabilitiesValue(marshalBytes)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)

	// Simulate data corruption by removing the first byte.
	corruptedBytes := marshalBytes[1:]
	_, err = decodeProbabilitiesValue(corruptedBytes)
	require.Error(t, err) // Expect an error
}

func TestDecodeThroughtputValue(t *testing.T) {
	expected := []*samplemodel.Throughput{
		{Service: "my-svc", Operation: "op"},
		{Service: "our-svc", Operation: "op2"},
	}

	marshalBytes, err := json.Marshal(expected)
	require.NoError(t, err)
	acrual, err := decodeThroughputValue(marshalBytes)
	require.NoError(t, err)
	assert.Equal(t, expected, acrual)
}

func runWithBadger(t *testing.T, test func(t *testing.T, store *SamplingStore)) {
	opts := badger.DefaultOptions("")

	opts.SyncWrites = false
	dir := t.TempDir()
	opts.Dir = dir
	opts.ValueDir = dir

	store, err := badger.Open(opts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
	}()
	ss := newTestSamplingStore(store)
	test(t, ss)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
