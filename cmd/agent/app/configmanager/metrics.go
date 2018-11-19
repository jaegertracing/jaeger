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
	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// configManagerMetrics holds metrics related to ClientConfigManager
type configManagerMetrics struct {
	// Number of successful sampling rate responses from collector
	SamplingSuccess metrics.Counter `metric:"collector-proxy" tags:"result=ok,endpoint=sampling"`

	// Number of failed sampling rate responses from collector
	SamplingFailures metrics.Counter `metric:"collector-proxy" tags:"result=err,endpoint=sampling"`

	// Number of successful baggage restriction responses from collector
	BaggageSuccess metrics.Counter `metric:"collector-proxy" tags:"result=ok,endpoint=baggage"`

	// Number of failed baggage restriction responses from collector
	BaggageFailures metrics.Counter `metric:"collector-proxy" tags:"result=err,endpoint=baggage"`
}

// ManagerWithMetrics is manager with metrics integration.
type ManagerWithMetrics struct {
	wrapped ClientConfigManager
	metrics configManagerMetrics
}

// WrapWithMetrics wraps ClientConfigManager and creates metrics for its invocations.
func WrapWithMetrics(manager ClientConfigManager, mFactory metrics.Factory) *ManagerWithMetrics {
	m := configManagerMetrics{}
	metrics.Init(&m, mFactory, nil)
	return &ManagerWithMetrics{wrapped: manager, metrics: m}
}

// GetSamplingStrategy returns sampling strategy from server.
func (m *ManagerWithMetrics) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	r, err := m.wrapped.GetSamplingStrategy(serviceName)
	if err != nil {
		m.metrics.SamplingFailures.Inc(1)
	} else {
		m.metrics.SamplingSuccess.Inc(1)
	}
	return r, err
}

// GetBaggageRestrictions returns baggage restrictions from server.
func (m *ManagerWithMetrics) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	r, err := m.wrapped.GetBaggageRestrictions(serviceName)
	if err != nil {
		m.metrics.BaggageFailures.Inc(1)
	} else {
		m.metrics.BaggageSuccess.Inc(1)
	}
	return r, err
}
