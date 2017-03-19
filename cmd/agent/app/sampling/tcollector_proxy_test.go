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

	"code.uber.internal/infra/jaeger/oss/cmd/agent/app/testutils"
)

func TestTCollectorProxy(t *testing.T) {
	metricsFactory, collector := testutils.InitMockCollector(t)
	defer collector.Close()

	collector.AddSamplingStrategy("service1", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
		RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: 10,
		}})

	mgr := NewTCollectorSamplingManagerProxy(collector.Channel, metricsFactory, nil)

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
	proxy := &tcollectorSamplingProxy{client: client}
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
