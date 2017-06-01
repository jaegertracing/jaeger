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
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/baggage"
	"github.com/uber/jaeger/thrift-gen/sampling"
)

type collectorProxy struct {
	samplingClient sampling.TChanSamplingManager
	baggageClient  baggage.TChanBaggageRestrictionManager
	metrics        struct {
		// Number of successful sampling rate responses from collector
		SamplingSuccess metrics.Counter `metric:"tc-sampling-proxy" tags:"result=ok,type=sampling"`

		// Number of failed sampling rate responses from collector
		SamplingFailures metrics.Counter `metric:"tc-sampling-proxy" tags:"result=err,type=sampling"`

		// Number of successful baggage restriction responses from collector
		BaggageSuccess metrics.Counter `metric:"tc-sampling-proxy" tags:"result=ok,type=baggage"`

		// Number of failed baggage restriction responses from collector
		BaggageFailures metrics.Counter `metric:"tc-sampling-proxy" tags:"result=err,type=baggage"`
	}
}

// NewCollectorProxy implements Manager by proxying the requests to collector.
func NewCollectorProxy(svc string, channel *tchannel.Channel, mFactory metrics.Factory) Manager {
	thriftClient := thrift.NewClient(channel, svc, nil)
	res := &collectorProxy{
		samplingClient: sampling.NewTChanSamplingManagerClient(thriftClient),
		baggageClient:  baggage.NewTChanBaggageRestrictionManagerClient(thriftClient),
	}
	metrics.Init(&res.metrics, mFactory, nil)
	return res
}

func (c *collectorProxy) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	ctx, cancel := tchannel.NewContextBuilder(time.Second).DisableTracing().Build()
	defer cancel()

	resp, err := c.samplingClient.GetSamplingStrategy(ctx, serviceName)
	if err != nil {
		c.metrics.SamplingFailures.Inc(1)
		return nil, err
	}
	c.metrics.SamplingSuccess.Inc(1)
	return resp, nil
}

func (c *collectorProxy) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	ctx, cancel := tchannel.NewContextBuilder(time.Second).DisableTracing().Build()
	defer cancel()

	resp, err := c.baggageClient.GetBaggageRestrictions(ctx, serviceName)
	if err != nil {
		c.metrics.BaggageFailures.Inc(1)
		return nil, err
	}
	c.metrics.BaggageSuccess.Inc(1)
	return resp, nil
}
