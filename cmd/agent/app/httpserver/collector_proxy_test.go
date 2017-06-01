// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
		{BaggageKey: "key", MaxValueLength: 10},
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
	assert.Equal(t, "key", bResp[0].BaggageKey)
	assert.EqualValues(t, 10, bResp[0].MaxValueLength)

	// must emit metrics
	mTestutils.AssertCounterMetrics(t, metricsFactory, []mTestutils.ExpectedMetric{
		{Name: "tc-sampling-proxy", Tags: map[string]string{"result": "ok", "type": "sampling"}, Value: 1},
		{Name: "tc-sampling-proxy", Tags: map[string]string{"result": "err", "type": "sampling"}, Value: 0},
		{Name: "tc-sampling-proxy", Tags: map[string]string{"result": "ok", "type": "baggage"}, Value: 1},
		{Name: "tc-sampling-proxy", Tags: map[string]string{"result": "err", "type": "baggage"}, Value: 0},
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
		{Name: "tc-sampling-proxy", Tags: map[string]string{"result": "err", "type": "sampling"}, Value: 1},
		{Name: "tc-sampling-proxy", Tags: map[string]string{"result": "err", "type": "baggage"}, Value: 1},
	}...)
}

type failingClient struct{}

func (c *failingClient) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("error")
}

func (c *failingClient) GetBaggageRestrictions(ctx thrift.Context, serviceName string) ([]*baggage.BaggageRestriction, error) {
	return nil, errors.New("error")
}
