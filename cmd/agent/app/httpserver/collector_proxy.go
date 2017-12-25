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
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type collectorProxy struct {
	samplingClient sampling.TChanSamplingManager
	baggageClient  baggage.TChanBaggageRestrictionManager
	metrics        struct {
		// Number of successful sampling rate responses from collector
		SamplingSuccess metrics.Counter `metric:"collector-proxy" tags:"result=ok,endpoint=sampling"`

		// Number of failed sampling rate responses from collector
		SamplingFailures metrics.Counter `metric:"collector-proxy" tags:"result=err,endpoint=sampling"`

		// Number of successful baggage restriction responses from collector
		BaggageSuccess metrics.Counter `metric:"collector-proxy" tags:"result=ok,endpoint=baggage"`

		// Number of failed baggage restriction responses from collector
		BaggageFailures metrics.Counter `metric:"collector-proxy" tags:"result=err,endpoint=baggage"`
	}
}

// NewCollectorProxy implements Manager by proxying the requests to collector.
func NewCollectorProxy(svc string, channel *tchannel.Channel, mFactory metrics.Factory) ClientConfigManager {
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

	// TODO: enable tracer on the tchannel and get metrics for free (sampler can be off)
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
