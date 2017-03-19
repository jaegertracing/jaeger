package sampling

import (
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/sampling"
)

type tcollectorSamplingProxy struct {
	client  sampling.TChanSamplingManager
	metrics struct {
		// Number of successful sampling rate responses from collector
		SamplingResponses metrics.Counter `metric:"tc-sampling-proxy.sampling.responses"`

		// Number of failed sampling rate responses from collector
		SamplingErrors metrics.Counter `metric:"tc-sampling-proxy.sampling.errors"`
	}
}

// NewTCollectorSamplingManagerProxy implements Manager by proxying the requests to tcollector.
func NewTCollectorSamplingManagerProxy(channel *tchannel.Channel, mFactory metrics.Factory, clientOpts *thrift.ClientOptions) Manager {
	thriftClient := thrift.NewClient(channel, "tcollector", clientOpts)
	client := sampling.NewTChanSamplingManagerClient(thriftClient)
	res := &tcollectorSamplingProxy{client: client}
	metrics.Init(&res.metrics, mFactory, nil)
	return res
}

func (c *tcollectorSamplingProxy) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	ctx, cancel := tchannel.NewContextBuilder(time.Second).DisableTracing().Build()
	defer cancel()

	resp, err := c.client.GetSamplingStrategy(ctx, serviceName)
	if err != nil {
		c.metrics.SamplingErrors.Inc(1)
		return nil, err
	}
	c.metrics.SamplingResponses.Inc(1)
	return resp, nil
}
