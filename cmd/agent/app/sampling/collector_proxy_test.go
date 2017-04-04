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

package sampling

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"
	"github.com/uber/jaeger/thrift-gen/sampling"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/cmd/agent/app/testutils"
)

func TestCollectorProxy(t *testing.T) {
	metricsFactory, collector := testutils.InitMockCollector(t)
	defer collector.Close()

	collector.AddSamplingStrategy("service1", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
		RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: 10,
		}})

	mgr := NewCollectorProxy("jaeger-collector", collector.Channel, metricsFactory, nil)

	resp, err := mgr.GetSamplingStrategy("service1")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.EqualValues(t, resp.StrategyType, sampling.SamplingStrategyType_RATE_LIMITING)
	require.NotNil(t, resp.RateLimitingSampling)
	require.EqualValues(t, 10, resp.RateLimitingSampling.MaxTracesPerSecond)

	// must emit metrics
	mTestutils.AssertCounterMetrics(t, metricsFactory, []mTestutils.ExpectedMetric{
		{Name: "tc-sampling-proxy.sampling.responses", Value: 1},
		{Name: "tc-sampling-proxy.sampling.errors", Value: 0},
	}...)
}

func TestTCollectorProxyClientErrorPropagates(t *testing.T) {
	mFactory := metrics.NewLocalFactory(time.Minute)
	client := &failingClient{}
	proxy := &collectorProxy{client: client}
	metrics.Init(&proxy.metrics, mFactory, nil)
	_, err := proxy.GetSamplingStrategy("test")
	assert.EqualError(t, err, "error")
	mTestutils.AssertCounterMetrics(t, mFactory,
		mTestutils.ExpectedMetric{Name: "tc-sampling-proxy.sampling.errors", Value: 1})
}

type failingClient struct{}

func (c *failingClient) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("error")
}
