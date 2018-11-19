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

package configmanager

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type noopManager struct {
}

func (noopManager) GetSamplingStrategy(s string) (*sampling.SamplingStrategyResponse, error) {
	if s == "failed" {
		return nil, errors.New("failed")
	}
	return &sampling.SamplingStrategyResponse{StrategyType: sampling.SamplingStrategyType_PROBABILISTIC}, nil
}
func (noopManager) GetBaggageRestrictions(s string) ([]*baggage.BaggageRestriction, error) {
	if s == "failed" {
		return nil, errors.New("failed")
	}
	return []*baggage.BaggageRestriction{{BaggageKey: "foo"}}, nil
}

func TestMetrics(t *testing.T) {
	tests := []struct {
		expected []mTestutils.ExpectedMetric
		err      error
	}{
		{expected: []mTestutils.ExpectedMetric{
			{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "sampling"}, Value: 1},
			{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "sampling"}, Value: 0},
			{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "baggage"}, Value: 1},
			{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "baggage"}, Value: 0},
		}},
		{expected: []mTestutils.ExpectedMetric{
			{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "sampling"}, Value: 0},
			{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "sampling"}, Value: 1},
			{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "baggage"}, Value: 0},
			{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "baggage"}, Value: 1},
		}, err: errors.New("failed")},
	}

	for _, test := range tests {
		metricsFactory := metrics.NewLocalFactory(time.Microsecond)
		mgr := WrapWithMetrics(&noopManager{}, metricsFactory)

		if test.err != nil {
			s, err := mgr.GetSamplingStrategy(test.err.Error())
			require.Nil(t, s)
			assert.EqualError(t, err, test.err.Error())
			b, err := mgr.GetBaggageRestrictions(test.err.Error())
			require.Nil(t, b)
			assert.EqualError(t, err, test.err.Error())
		} else {
			s, err := mgr.GetSamplingStrategy("")
			require.NoError(t, err)
			require.NotNil(t, s)
			b, err := mgr.GetBaggageRestrictions("")
			require.NoError(t, err)
			require.NotNil(t, b)
		}
		mTestutils.AssertCounterMetrics(t, metricsFactory, test.expected...)
	}
}
