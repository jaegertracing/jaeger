// Copyright (c) 2018 The Jaeger Authors.
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

package static

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// strategiesJSON returns the strategy with
// a given probability.
func strategiesJSON(probability float32) string {
	strategy := fmt.Sprintf(`
		{
			"default_strategy": {
			"type": "probabilistic",
			"param": 0.5
			},
			"service_strategies": [
			{
				"service": "foo",
				"type": "probabilistic",
				"param": %.1f
			},
			{
				"service": "bar",
				"type": "ratelimiting",
				"param": 5
			}
			]
		}
		`,
		probability,
	)
	return strategy
}

// Returns strategies in JSON format. Used for testing
// URL option for sampling strategies.
func mockStrategyServer() (*httptest.Server, *atomic.String) {
	strategy := atomic.NewString(strategiesJSON(0.8))
	f := func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-content":
			w.Write([]byte("bad-content"))
			return

		case "/bad-status":
			w.WriteHeader(404)
			return

		case "/service-unavailable":
			w.WriteHeader(503)
			return

		default:
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(strategy.Load()))
		}
	}
	return httptest.NewServer(http.HandlerFunc(f)), strategy
}

func TestStrategyStoreWithFile(t *testing.T) {
	_, err := NewStrategyStore(Options{StrategiesFile: "fileNotFound.json"}, zap.NewNop())
	assert.Contains(t, err.Error(), "failed to read strategies file fileNotFound.json")

	_, err = NewStrategyStore(Options{StrategiesFile: "fixtures/bad_strategies.json"}, zap.NewNop())
	assert.EqualError(t, err,
		"failed to unmarshal strategies: json: cannot unmarshal string into Go value of type static.strategies")

	// Test default strategy
	logger, buf := testutils.NewLogger()
	store, err := NewStrategyStore(Options{}, logger)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No sampling strategies provided or URL is unavailable, using defaults")
	s, err := store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.001), *s)

	// Test reading strategies from a file
	store, err = NewStrategyStore(Options{StrategiesFile: "fixtures/strategies.json"}, logger)
	require.NoError(t, err)
	s, err = store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8), *s)

	s, err = store.GetSamplingStrategy(context.Background(), "bar")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 5), *s)

	s, err = store.GetSamplingStrategy(context.Background(), "default")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.5), *s)
}

func TestStrategyStoreWithURL(t *testing.T) {
	// Test default strategy when URL is temporarily unavailable.
	logger, buf := testutils.NewLogger()
	mockServer, _ := mockStrategyServer()
	store, err := NewStrategyStore(Options{StrategiesFile: mockServer.URL + "/service-unavailable"}, logger)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No sampling strategies provided or URL is unavailable, using defaults")
	s, err := store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.001), *s)

	// Test downloading strategies from a URL.
	store, err = NewStrategyStore(Options{StrategiesFile: mockServer.URL}, logger)
	require.NoError(t, err)

	s, err = store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8), *s)

	s, err = store.GetSamplingStrategy(context.Background(), "bar")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 5), *s)
}

func TestPerOperationSamplingStrategies(t *testing.T) {
	logger, buf := testutils.NewLogger()
	store, err := NewStrategyStore(Options{StrategiesFile: "fixtures/operation_strategies.json"}, logger)
	assert.Contains(t, buf.String(), "Operation strategies only supports probabilistic sampling at the moment,"+
		"'op2' defaulting to probabilistic sampling with probability 0.8")
	assert.Contains(t, buf.String(), "Operation strategies only supports probabilistic sampling at the moment,"+
		"'op4' defaulting to probabilistic sampling with probability 0.001")
	require.NoError(t, err)

	expected := makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8)

	s, err := store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, s.StrategyType)
	assert.Equal(t, *expected.ProbabilisticSampling, *s.ProbabilisticSampling)

	require.NotNil(t, s.OperationSampling)
	os := s.OperationSampling
	assert.EqualValues(t, os.DefaultSamplingProbability, 0.8)
	require.Len(t, os.PerOperationStrategies, 4)

	assert.Equal(t, "op6", os.PerOperationStrategies[0].Operation)
	assert.EqualValues(t, 0.5, os.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op1", os.PerOperationStrategies[1].Operation)
	assert.EqualValues(t, 0.2, os.PerOperationStrategies[1].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op0", os.PerOperationStrategies[2].Operation)
	assert.EqualValues(t, 0.2, os.PerOperationStrategies[2].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op7", os.PerOperationStrategies[3].Operation)
	assert.EqualValues(t, 1, os.PerOperationStrategies[3].ProbabilisticSampling.SamplingRate)

	expected = makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 5)

	s, err = store.GetSamplingStrategy(context.Background(), "bar")
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_RATE_LIMITING, s.StrategyType)
	assert.Equal(t, *expected.RateLimitingSampling, *s.RateLimitingSampling)

	require.NotNil(t, s.OperationSampling)
	os = s.OperationSampling
	assert.EqualValues(t, os.DefaultSamplingProbability, 0.001)
	require.Len(t, os.PerOperationStrategies, 5)
	assert.Equal(t, "op3", os.PerOperationStrategies[0].Operation)
	assert.EqualValues(t, 0.3, os.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op5", os.PerOperationStrategies[1].Operation)
	assert.EqualValues(t, 0.4, os.PerOperationStrategies[1].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op0", os.PerOperationStrategies[2].Operation)
	assert.EqualValues(t, 0.2, os.PerOperationStrategies[2].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op6", os.PerOperationStrategies[3].Operation)
	assert.EqualValues(t, 0, os.PerOperationStrategies[3].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op7", os.PerOperationStrategies[4].Operation)
	assert.EqualValues(t, 1, os.PerOperationStrategies[4].ProbabilisticSampling.SamplingRate)

	s, err = store.GetSamplingStrategy(context.Background(), "default")
	require.NoError(t, err)
	expectedRsp := makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.5)
	expectedRsp.OperationSampling = &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability: 0.5,
		PerOperationStrategies: []*sampling.OperationSamplingStrategy{
			{
				Operation: "op0",
				ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
					SamplingRate: 0.2,
				},
			},
			{
				Operation: "op6",
				ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
					SamplingRate: 0,
				},
			},
			{
				Operation: "op7",
				ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
					SamplingRate: 1,
				},
			},
		},
	}
	assert.EqualValues(t, expectedRsp, *s)
}

func TestMissingServiceSamplingStrategyTypes(t *testing.T) {
	logger, buf := testutils.NewLogger()
	store, err := NewStrategyStore(Options{StrategiesFile: "fixtures/missing-service-types.json"}, logger)
	assert.Contains(t, buf.String(), "Failed to parse sampling strategy")
	require.NoError(t, err)

	expected := makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, defaultSamplingProbability)

	s, err := store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, s.StrategyType)
	assert.Equal(t, *expected.ProbabilisticSampling, *s.ProbabilisticSampling)

	require.NotNil(t, s.OperationSampling)
	os := s.OperationSampling
	assert.EqualValues(t, os.DefaultSamplingProbability, defaultSamplingProbability)
	require.Len(t, os.PerOperationStrategies, 1)
	assert.Equal(t, "op1", os.PerOperationStrategies[0].Operation)
	assert.EqualValues(t, 0.2, os.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate)

	expected = makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, defaultSamplingProbability)

	s, err = store.GetSamplingStrategy(context.Background(), "bar")
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, s.StrategyType)
	assert.Equal(t, *expected.ProbabilisticSampling, *s.ProbabilisticSampling)

	require.NotNil(t, s.OperationSampling)
	os = s.OperationSampling
	assert.EqualValues(t, os.DefaultSamplingProbability, 0.001)
	require.Len(t, os.PerOperationStrategies, 2)
	assert.Equal(t, "op3", os.PerOperationStrategies[0].Operation)
	assert.EqualValues(t, 0.3, os.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate)
	assert.Equal(t, "op5", os.PerOperationStrategies[1].Operation)
	assert.EqualValues(t, 0.4, os.PerOperationStrategies[1].ProbabilisticSampling.SamplingRate)

	s, err = store.GetSamplingStrategy(context.Background(), "default")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.5), *s)
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		strategy serviceStrategy
		expected sampling.SamplingStrategyResponse
	}{
		{
			strategy: serviceStrategy{
				Service:  "svc",
				strategy: strategy{Type: "probabilistic", Param: 0.2},
			},
			expected: makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.2),
		},
		{
			strategy: serviceStrategy{
				Service:  "svc",
				strategy: strategy{Type: "ratelimiting", Param: 3.5},
			},
			expected: makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 3),
		},
	}
	logger, buf := testutils.NewLogger()
	store := &strategyStore{logger: logger}
	for _, test := range tests {
		tt := test
		t.Run("", func(t *testing.T) {
			assert.EqualValues(t, tt.expected, *store.parseStrategy(&tt.strategy.strategy))
		})
	}
	assert.Empty(t, buf.String())

	// Test nonexistent strategy type
	actual := *store.parseStrategy(&strategy{Type: "blah", Param: 3.5})
	expected := makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, defaultSamplingProbability)
	assert.EqualValues(t, expected, actual)
	assert.Contains(t, buf.String(), "Failed to parse sampling strategy")
}

func makeResponse(samplerType sampling.SamplingStrategyType, param float64) (resp sampling.SamplingStrategyResponse) {
	resp.StrategyType = samplerType
	if samplerType == sampling.SamplingStrategyType_PROBABILISTIC {
		resp.ProbabilisticSampling = &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: param,
		}
	} else if samplerType == sampling.SamplingStrategyType_RATE_LIMITING {
		resp.RateLimitingSampling = &sampling.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: int16(param),
		}
	}
	return resp
}

func TestDeepCopy(t *testing.T) {
	s := &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: 0.5,
		},
	}
	cp := deepCopy(s)
	assert.False(t, cp == s)
	assert.EqualValues(t, cp, s)
}

func TestAutoUpdateStrategyWithFile(t *testing.T) {
	tempFile, _ := ioutil.TempFile("", "for_go_test_*.json")
	require.NoError(t, tempFile.Close())
	defer func() {
		require.NoError(t, os.Remove(tempFile.Name()))
	}()

	// copy known fixture content into temp file which we can later overwrite
	srcFile, dstFile := "fixtures/strategies.json", tempFile.Name()
	srcBytes, err := ioutil.ReadFile(srcFile)
	require.NoError(t, err)
	require.NoError(t, ioutil.WriteFile(dstFile, srcBytes, 0644))

	ss, err := NewStrategyStore(Options{
		StrategiesFile: dstFile,
		ReloadInterval: time.Millisecond * 10,
	}, zap.NewNop())
	require.NoError(t, err)
	store := ss.(*strategyStore)
	defer store.Close()

	// confirm baseline value
	s, err := store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8), *s)

	// verify that reloading in no-op
	value := store.reloadSamplingStrategy(samplingStrategyLoader(dstFile), string(srcBytes))
	assert.Equal(t, string(srcBytes), value)

	// update file with new probability of 0.9
	newStr := strings.Replace(string(srcBytes), "0.8", "0.9", 1)
	require.NoError(t, ioutil.WriteFile(dstFile, []byte(newStr), 0644))

	// wait for reload timer
	for i := 0; i < 1000; i++ { // wait up to 1sec
		s, err = store.GetSamplingStrategy(context.Background(), "foo")
		require.NoError(t, err)
		if s.ProbabilisticSampling != nil && s.ProbabilisticSampling.SamplingRate == 0.9 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.9), *s)
}

func TestAutoUpdateStrategyWithURL(t *testing.T) {
	mockServer, mockStrategy := mockStrategyServer()
	ss, err := NewStrategyStore(Options{
		StrategiesFile: mockServer.URL,
		ReloadInterval: 10 * time.Millisecond,
	}, zap.NewNop())
	require.NoError(t, err)
	store := ss.(*strategyStore)
	defer store.Close()

	// confirm baseline value
	s, err := store.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8), *s)

	// verify that reloading in no-op
	value := store.reloadSamplingStrategy(samplingStrategyLoader(mockServer.URL), mockStrategy.Load())
	assert.Equal(t, mockStrategy.Load(), value)

	// update original strategies with new probability of 0.9
	mockStrategy.Store(strategiesJSON(0.9))

	// wait for reload timer
	for i := 0; i < 1000; i++ { // wait up to 1sec
		s, err = store.GetSamplingStrategy(context.Background(), "foo")
		require.NoError(t, err)
		if s.ProbabilisticSampling != nil && s.ProbabilisticSampling.SamplingRate == 0.9 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.9), *s)
}

func TestAutoUpdateStrategyErrors(t *testing.T) {
	tempFile, _ := ioutil.TempFile("", "for_go_test_*.json")
	require.NoError(t, tempFile.Close())
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	s, err := NewStrategyStore(Options{
		StrategiesFile: "fixtures/strategies.json",
		ReloadInterval: time.Hour,
	}, logger)
	require.NoError(t, err)
	store := s.(*strategyStore)
	defer store.Close()

	// check invalid file path or read failure
	assert.Equal(t, "blah", store.reloadSamplingStrategy(samplingStrategyLoader(tempFile.Name()+"bad-path"), "blah"))
	assert.Len(t, logs.FilterMessage("failed to re-load sampling strategies").All(), 1)

	// check bad file content
	require.NoError(t, ioutil.WriteFile(tempFile.Name(), []byte("bad value"), 0644))
	assert.Equal(t, "blah", store.reloadSamplingStrategy(samplingStrategyLoader(tempFile.Name()), "blah"))
	assert.Len(t, logs.FilterMessage("failed to update sampling strategies").All(), 1)

	// check invalid url
	assert.Equal(t, "duh", store.reloadSamplingStrategy(samplingStrategyLoader("bad-url"), "duh"))
	assert.Len(t, logs.FilterMessage("failed to re-load sampling strategies").All(), 2)

	// check status code other than 200
	mockServer, _ := mockStrategyServer()
	assert.Equal(t, "duh", store.reloadSamplingStrategy(samplingStrategyLoader(mockServer.URL+"/bad-status"), "duh"))
	assert.Len(t, logs.FilterMessage("failed to re-load sampling strategies").All(), 3)

	// check bad content from url
	assert.Equal(t, "duh", store.reloadSamplingStrategy(samplingStrategyLoader(mockServer.URL+"/bad-content"), "duh"))
	assert.Len(t, logs.FilterMessage("failed to update sampling strategies").All(), 2)
}

func TestServiceNoPerOperationStrategies(t *testing.T) {
	store, err := NewStrategyStore(Options{StrategiesFile: "fixtures/service_no_per_operation.json"}, zap.NewNop())
	require.NoError(t, err)

	s, err := store.GetSamplingStrategy(context.Background(), "ServiceA")
	require.NoError(t, err)
	assert.Equal(t, 1.0, s.OperationSampling.DefaultSamplingProbability)

	s, err = store.GetSamplingStrategy(context.Background(), "ServiceB")
	require.NoError(t, err)

	expected := makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 3)
	assert.Equal(t, *expected.RateLimitingSampling, *s.RateLimitingSampling)
}

func TestSamplingStrategyLoader(t *testing.T) {
	// invalid file path
	loader := samplingStrategyLoader("not-exists")
	_, err := loader()
	assert.Contains(t, err.Error(), "failed to read strategies file not-exists")

	// status code other than 200
	mockServer, _ := mockStrategyServer()
	loader = samplingStrategyLoader(mockServer.URL + "/bad-status")
	_, err = loader()
	assert.Contains(t, err.Error(), "receiving 404 Not Found while downloading strategies file")

	// should download content from URL
	loader = samplingStrategyLoader(mockServer.URL + "/bad-content")
	content, err := loader()
	require.NoError(t, err)
	assert.Equal(t, "bad-content", string(content))
}
