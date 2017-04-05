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

package testutils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/sampling"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

func withTCollector(t *testing.T, fn func(collector *MockTCollector, ctx thrift.Context)) {
	_, collector := InitMockCollector(t)
	defer collector.Close()

	time.Sleep(10 * time.Millisecond) // give the server a chance to start

	ctx, ctxCancel := thrift.NewContext(time.Second)
	defer ctxCancel()

	fn(collector, ctx)
}

func withSamplingClient(t *testing.T, fn func(collector *MockTCollector, ctx thrift.Context, client sampling.TChanSamplingManager)) {
	withTCollector(t, func(collector *MockTCollector, ctx thrift.Context) {
		thriftClient := thrift.NewClient(collector.Channel, "jaeger-collector", nil)
		client := sampling.NewTChanSamplingManagerClient(thriftClient)

		fn(collector, ctx, client)
	})
}

func withZipkinClient(t *testing.T, fn func(collector *MockTCollector, ctx thrift.Context, client zipkincore.TChanZipkinCollector)) {
	withTCollector(t, func(collector *MockTCollector, ctx thrift.Context) {
		thriftClient := thrift.NewClient(collector.Channel, "jaeger-collector", nil)
		client := zipkincore.NewTChanZipkinCollectorClient(thriftClient)

		fn(collector, ctx, client)
	})
}

func withJaegerClient(t *testing.T, fn func(collector *MockTCollector, ctx thrift.Context, client jaeger.TChanCollector)) {
	withTCollector(t, func(collector *MockTCollector, ctx thrift.Context) {
		thriftClient := thrift.NewClient(collector.Channel, "jaeger-collector", nil)
		client := jaeger.NewTChanCollectorClient(thriftClient)

		fn(collector, ctx, client)
	})
}

func TestMockTCollectorSampling(t *testing.T) {
	withSamplingClient(t, func(collector *MockTCollector, ctx thrift.Context, client sampling.TChanSamplingManager) {
		s, err := client.GetSamplingStrategy(ctx, "default-service")
		require.NoError(t, err)
		require.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, s.StrategyType)
		require.NotNil(t, s.ProbabilisticSampling)
		assert.Equal(t, 0.01, s.ProbabilisticSampling.SamplingRate)

		collector.AddSamplingStrategy("service1", &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: 10,
			}})

		s, err = client.GetSamplingStrategy(ctx, "service1")
		require.NoError(t, err)
		require.Equal(t, sampling.SamplingStrategyType_RATE_LIMITING, s.StrategyType)
		require.NotNil(t, s.RateLimitingSampling)
		assert.EqualValues(t, 10, s.RateLimitingSampling.MaxTracesPerSecond)
	})
}

func TestMockTCollectorZipkin(t *testing.T) {
	withZipkinClient(t, func(collector *MockTCollector, ctx thrift.Context, client zipkincore.TChanZipkinCollector) {
		span := &zipkincore.Span{Name: "service3"}
		_, err := client.SubmitZipkinBatch(ctx, []*zipkincore.Span{span})
		require.NoError(t, err)
		spans := collector.GetZipkinSpans()
		require.Equal(t, 1, len(spans))
		assert.Equal(t, "service3", spans[0].Name)

		collector.ReturnErr = true
		_, err = client.SubmitZipkinBatch(ctx, []*zipkincore.Span{span})
		assert.Error(t, err)
	})
}

func TestMockTCollector(t *testing.T) {
	withJaegerClient(t, func(collector *MockTCollector, ctx thrift.Context, client jaeger.TChanCollector) {
		batch := &jaeger.Batch{
			Spans: []*jaeger.Span{
				{OperationName: "service4"},
			},
			Process: &jaeger.Process{
				ServiceName: "someServiceName",
			},
		}
		_, err := client.SubmitBatches(ctx, []*jaeger.Batch{batch})
		require.NoError(t, err)
		batches := collector.GetJaegerBatches()
		require.Equal(t, 1, len(batches))
		assert.Equal(t, "service4", batches[0].Spans[0].OperationName)
		assert.Equal(t, "someServiceName", batches[0].Process.ServiceName)

		collector.ReturnErr = true
		_, err = client.SubmitBatches(ctx, []*jaeger.Batch{batch})
		assert.Error(t, err)
	})
}

func TestMockTCollectorErrors(t *testing.T) {
	_, err := startMockTCollector("", "127.0.0.1:0")
	assert.Error(t, err, "error because of empty service name")

	_, err = startMockTCollector("test", "127.0.0:0")
	assert.Error(t, err, "error because of bad address")
}
