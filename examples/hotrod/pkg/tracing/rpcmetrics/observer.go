// Copyright (c) 2023 The Jaeger Authors.
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

package rpcmetrics

import (
	"context"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

const defaultMaxNumberOfEndpoints = 200

var _ sdktrace.SpanProcessor = (*Observer)(nil)

// Observer is an observer that can emit RPC metrics.
type Observer struct {
	metricsByEndpoint *MetricsByEndpoint
}

// NewObserver creates a new observer that can emit RPC metrics.
func NewObserver(metricsFactory metrics.Factory, normalizer NameNormalizer) *Observer {
	return &Observer{
		metricsByEndpoint: newMetricsByEndpoint(
			metricsFactory,
			normalizer,
			defaultMaxNumberOfEndpoints,
		),
	}
}

func (o *Observer) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {}

func (o *Observer) OnEnd(sp sdktrace.ReadOnlySpan) {
	operationName := sp.Name()
	if operationName == "" {
		return
	}
	if sp.SpanKind() != trace.SpanKindServer {
		return
	}

	mets := o.metricsByEndpoint.get(operationName)
	latency := sp.EndTime().Sub(sp.StartTime())

	if status := sp.Status(); status.Code == codes.Error {
		mets.RequestCountFailures.Inc(1)
		mets.RequestLatencyFailures.Record(latency)
	} else {
		mets.RequestCountSuccess.Inc(1)
		mets.RequestLatencySuccess.Record(latency)
	}
	for _, attr := range sp.Attributes() {
		if string(attr.Key) == string(semconv.HTTPStatusCodeKey) {
			if attr.Value.Type() == attribute.INT64 {
				mets.recordHTTPStatusCode(attr.Value.AsInt64())
			} else if attr.Value.Type() == attribute.STRING {
				s := attr.Value.AsString()
				if n, err := strconv.Atoi(s); err == nil {
					mets.recordHTTPStatusCode(int64(n))
				}
			}
		}
	}
}

func (o *Observer) Shutdown(ctx context.Context) error {
	return nil
}

func (o *Observer) ForceFlush(ctx context.Context) error {
	return nil
}
