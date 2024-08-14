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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
)

func withPopulatedSamplingStore(f func(samplingStore *SamplingStore)) {
	now := time.Now()
	millisAfter := now.Add(time.Millisecond * time.Duration(100))
	secondsAfter := now.Add(time.Second * time.Duration(2))
	throughputs := []*storedThroughput{
		{[]*model.Throughput{{Service: "svc-1", Operation: "op-1", Count: 1}}, now},
		{[]*model.Throughput{{Service: "svc-1", Operation: "op-2", Count: 1}}, millisAfter},
		{[]*model.Throughput{{Service: "svc-2", Operation: "op-3", Count: 1}}, secondsAfter},
	}
	pQPS := &storedServiceOperationProbabilitiesAndQPS{
		hostname: "guntur38ab8928", probabilities: model.ServiceOperationProbabilities{"svc-1": {"op-1": 0.01}}, qps: model.ServiceOperationQPS{"svc-1": {"op-1": 10.0}}, time: now,
	}
	samplingStore := &SamplingStore{throughputs: throughputs, probabilitiesAndQPS: pQPS}
	f(samplingStore)
}

func withMemorySamplingStore(f func(samplingStore *SamplingStore)) {
	f(NewSamplingStore(5))
}

func TestInsertThroughtput(t *testing.T) {
	withMemorySamplingStore(func(samplingStore *SamplingStore) {
		start := time.Now()
		throughputs := []*model.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		require.NoError(t, samplingStore.InsertThroughput(throughputs))
		ret, _ := samplingStore.GetThroughput(start, start.Add(time.Second*time.Duration(1)))
		assert.Len(t, ret, 2)

		for i := 0; i < 10; i++ {
			in := []*model.Throughput{
				{Service: fmt.Sprint("svc-", i), Operation: fmt.Sprint("op-", i)},
			}
			samplingStore.InsertThroughput(in)
		}
		assert.Len(t, samplingStore.throughputs, 5)
	})
}

func TestGetThroughput(t *testing.T) {
	withPopulatedSamplingStore(func(samplingStore *SamplingStore) {
		start := time.Now()
		ret, err := samplingStore.GetThroughput(start, start.Add(time.Second*time.Duration(1)))
		require.NoError(t, err)
		assert.Len(t, ret, 1)
		ret1, _ := samplingStore.GetThroughput(start, start)
		assert.Empty(t, ret1)
		ret2, _ := samplingStore.GetThroughput(start, start.Add(time.Hour*time.Duration(1)))
		assert.Len(t, ret2, 2)
	})
}

func TestInsertProbabilitiesAndQPS(t *testing.T) {
	withMemorySamplingStore(func(samplingStore *SamplingStore) {
		require.NoError(t, samplingStore.InsertProbabilitiesAndQPS("dell11eg843d", model.ServiceOperationProbabilities{"new-srv": {"op": 0.1}}, model.ServiceOperationQPS{"new-srv": {"op": 4}}))
		assert.NotEmpty(t, 1, samplingStore.probabilitiesAndQPS)
		// Only latest one is kept in memory
		require.NoError(t, samplingStore.InsertProbabilitiesAndQPS("lncol73", model.ServiceOperationProbabilities{"my-app": {"hello": 0.3}}, model.ServiceOperationQPS{"new-srv": {"op": 7}}))
		assert.Equal(t, 0.3, samplingStore.probabilitiesAndQPS.probabilities["my-app"]["hello"])
	})
}

func TestGetLatestProbability(t *testing.T) {
	withMemorySamplingStore(func(samplingStore *SamplingStore) {
		// No priod data
		ret, err := samplingStore.GetLatestProbabilities()
		require.NoError(t, err)
		assert.Empty(t, ret)
	})

	withPopulatedSamplingStore(func(samplingStore *SamplingStore) {
		// With some pregenerated data
		ret, err := samplingStore.GetLatestProbabilities()
		require.NoError(t, err)
		assert.Equal(t, model.ServiceOperationProbabilities{"svc-1": {"op-1": 0.01}}, ret)
		require.NoError(t, samplingStore.InsertProbabilitiesAndQPS("utfhyolf", model.ServiceOperationProbabilities{"another-service": {"hello": 0.009}}, model.ServiceOperationQPS{"new-srv": {"op": 5}}))
		ret, _ = samplingStore.GetLatestProbabilities()
		assert.NotEqual(t, model.ServiceOperationProbabilities{"svc-1": {"op-1": 0.01}}, ret)
	})
}
