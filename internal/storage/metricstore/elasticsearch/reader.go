// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

type (
	// MetricsReader is a Elasticsearch metrics reader.
	MetricsReader struct {
		logger *zap.Logger
		tracer trace.Tracer
	}
	errMetricsQueryNotImplementedError struct{}
)

const (
	minStep = time.Millisecond
)

var ErrNotImplemented = &errMetricsQueryNotImplementedError{}

func (*errMetricsQueryNotImplementedError) Error() string {
	return "metrics querying is currently not implemented yet"
}

func NewMetricsReader(cfg config.Configuration, logger *zap.Logger, tracer trace.TracerProvider) (*MetricsReader, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &MetricsReader{
		logger: logger,
		tracer: tracer.Tracer("jaeger-elasticsearch-metrics"),
	}, nil
}

func (MetricsReader) GetLatencies(_ context.Context, _ *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

func (MetricsReader) GetCallRates(_ context.Context, _ *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

func (MetricsReader) GetErrorRates(_ context.Context, _ *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

func (MetricsReader) GetMinStepDuration(_ context.Context, _ *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
}
