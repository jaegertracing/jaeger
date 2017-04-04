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
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/sampling"
)

type collectorProxy struct {
	client  sampling.TChanSamplingManager
	metrics struct {
		// Number of successful sampling rate responses from collector
		SamplingResponses metrics.Counter `metric:"tc-sampling-proxy.sampling.responses"`

		// Number of failed sampling rate responses from collector
		SamplingErrors metrics.Counter `metric:"tc-sampling-proxy.sampling.errors"`
	}
}

// NewCollectorProxy implements Manager by proxying the requests to collector.
func NewCollectorProxy(svc string, channel *tchannel.Channel, mFactory metrics.Factory, clientOpts *thrift.ClientOptions) Manager {
	thriftClient := thrift.NewClient(channel, svc, clientOpts)
	client := sampling.NewTChanSamplingManagerClient(thriftClient)
	res := &collectorProxy{client: client}
	metrics.Init(&res.metrics, mFactory, nil)
	return res
}

func (c *collectorProxy) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
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
