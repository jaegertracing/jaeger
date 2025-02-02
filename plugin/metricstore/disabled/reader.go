// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package disabled

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/v1/metricstore"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

type (
	// MetricsReader represents a "disabled" metricstore.Reader implementation where
	// the METRICS_STORAGE_TYPE has not been set.
	MetricsReader struct{}

	// errMetricsQueryDisabledError is the error returned by disabledMetricsQueryService.
	errMetricsQueryDisabledError struct{}
)

// ErrDisabled is the error returned by a "disabled" MetricsQueryService on all of its endpoints.
var ErrDisabled = &errMetricsQueryDisabledError{}

func (*errMetricsQueryDisabledError) Error() string {
	return "metrics querying is currently disabled"
}

// NewMetricsReader returns a new Disabled MetricsReader.
func NewMetricsReader() (*MetricsReader, error) {
	return &MetricsReader{}, nil
}

// GetLatencies gets the latency metrics for the given set of latency query parameters.
func (*MetricsReader) GetLatencies(context.Context, *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetCallRates gets the call rate metrics for the given set of call rate query parameters.
func (*MetricsReader) GetCallRates(context.Context, *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetErrorRates gets the error rate metrics for the given set of error rate query parameters.
func (*MetricsReader) GetErrorRates(context.Context, *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetMinStepDuration gets the minimum step duration (the smallest possible duration between two data points in a time series) supported.
func (*MetricsReader) GetMinStepDuration(context.Context, *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return 0, ErrDisabled
}
