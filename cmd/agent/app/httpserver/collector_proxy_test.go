// Copyright (c) 2017 Uber Technologies, Inc.
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

package httpserver

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/cmd/agent/app/testutils"
	"github.com/uber/jaeger/thrift-gen/baggage"
	"github.com/uber/jaeger/thrift-gen/sampling"
)

func TestCollectorProxy(t *testing.T) {
	metricsFactory, collector := testutils.InitMockCollector(t)
	defer collector.Close()

	collector.AddSamplingStrategy("service1", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
		RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: 10,
		}})
	collector.AddBaggageRestrictions("service1", []*baggage.BaggageRestriction{
		{BaggageKey: "luggage", MaxValueLength: 10},
	})

	mgr := NewCollectorProxy("jaeger-collector", collector.Channel, metricsFactory)

	sResp, err := mgr.GetSamplingStrategy("service1")
	require.NoError(t, err)
	require.NotNil(t, sResp)
	assert.EqualValues(t, sResp.StrategyType, sampling.SamplingStrategyType_RATE_LIMITING)
	require.NotNil(t, sResp.RateLimitingSampling)
	assert.EqualValues(t, 10, sResp.RateLimitingSampling.MaxTracesPerSecond)

	bResp, err := mgr.GetBaggageRestrictions("service1")
	require.NoError(t, err)
	require.Len(t, bResp, 1)
	assert.Equal(t, "luggage", bResp[0].BaggageKey)
	assert.EqualValues(t, 10, bResp[0].MaxValueLength)

	// must emit metrics
	mTestutils.AssertCounterMetrics(t, metricsFactory, []mTestutils.ExpectedMetric{
		{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "sampling"}, Value: 1},
		{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "sampling"}, Value: 0},
		{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "baggage"}, Value: 1},
		{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "baggage"}, Value: 0},
	}...)
}

func TestTCollectorProxyClientErrorPropagates(t *testing.T) {
	mFactory := metrics.NewLocalFactory(time.Minute)
	proxy := &collectorProxy{samplingClient: &failingClient{}, baggageClient: &failingClient{}}
	metrics.Init(&proxy.metrics, mFactory, nil)
	_, err := proxy.GetSamplingStrategy("test")
	require.EqualError(t, err, "error")
	_, err = proxy.GetBaggageRestrictions("test")
	require.EqualError(t, err, "error")
	mTestutils.AssertCounterMetrics(t, mFactory, []mTestutils.ExpectedMetric{
		{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "sampling"}, Value: 1},
		{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "baggage"}, Value: 1},
	}...)
}

type failingClient struct{}

func (c *failingClient) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("error")
}

func (c *failingClient) GetBaggageRestrictions(ctx thrift.Context, serviceName string) ([]*baggage.BaggageRestriction, error) {
	return nil, errors.New("error")
}
