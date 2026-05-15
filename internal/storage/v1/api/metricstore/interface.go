// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

// Reader can load aggregated trace metrics from storage.
type Reader interface {
	// GetLatencies gets the latency metrics for a specific quantile (e.g. 0.99) and list of services
	// grouped by service and optionally grouped by operation.
	GetLatencies(ctx context.Context, params *LatenciesQueryParameters) (*metrics.MetricFamily, error)
	// GetCallRates gets the call rate metrics for a given list of services grouped by service
	// and optionally grouped by operation.
	GetCallRates(ctx context.Context, params *CallRateQueryParameters) (*metrics.MetricFamily, error)
	// GetErrorRates gets the error rate metrics for a given list of services grouped by service
	// and optionally grouped by operation.
	GetErrorRates(ctx context.Context, params *ErrorRateQueryParameters) (*metrics.MetricFamily, error)
	// GetDimensions returns the set of pre-configured dimensions on which
	// GetLatencies/GetCallRates/GetErrorRates may be filtered via
	// BaseQueryParameters.Filters. Backends that do not support dimension
	// filtering return an empty slice (or nil).
	GetDimensions(ctx context.Context) ([]Dimension, error)
}

// Dimension declares a Prometheus label (or equivalent backend field) that the
// SPM API can filter on via BaseQueryParameters.Filters. Dimensions are
// pre-configured by the operator — free-form/arbitrary tag filtering is
// intentionally not supported because Prometheus requires labels to be
// declared up front via the spanmetrics connector's "dimensions" config.
type Dimension struct {
	// Name is the OTel attribute name as declared in the spanmetrics connector
	// (e.g. "deployment.environment"). Dots are converted to underscores when
	// constructing PromQL label selectors.
	Name string `json:"name" mapstructure:"name"`
	// DisplayName is the user-facing label shown in the UI dropdown.
	// Defaults to Name when empty.
	DisplayName string `json:"displayName,omitempty" mapstructure:"display_name"`
	// Values is the closed set of allowed values shown in the UI dropdown
	// and enforced by the API. An empty slice means the UI may render a
	// free-text input; the API will still validate values against a small
	// set of PromQL-safe characters server-side.
	Values []string `json:"values,omitempty" mapstructure:"values"`
}

// BaseQueryParameters contains the common set of parameters used by all metrics queries:
// latency, call rate or error rate.
type BaseQueryParameters struct {
	// ServiceNames are the service names to fetch metrics from. The results will be grouped by service_name.
	ServiceNames []string
	// GroupByOperation determines if the metrics returned should be grouped by operation.
	GroupByOperation bool
	// EndTime is the ending time of the time series query range.
	EndTime *time.Time
	// Lookback is the duration from the end_time to look back on for metrics data points.
	// For example, if set to 1h, the query would span from end_time-1h to end_time.
	Lookback *time.Duration
	// Step size is the duration between data points of the query results.
	// For example, if set to 5s, the results would produce a data point every 5 seconds from the (EndTime - Lookback) to EndTime.
	Step *time.Duration
	// RatePer is the duration in which the per-second rate of change is calculated for a cumulative counter metric.
	RatePer *time.Duration
	// SpanKinds is the list of span kinds to include (logical OR) in the resulting metrics aggregation.
	// The jaeger_query extension always populates this with a non-empty default,
	// so backend implementations can assume it will not be empty.
	SpanKinds []string
	// Filters is an optional map of pre-configured dimension name -> value used
	// to scope the metrics query further (e.g. {"deployment.environment": "prod"}).
	// Keys are OTel attribute names matching a configured Dimension; backends that
	// do not support dimension filtering ignore this field.
	Filters map[string]string
}

// LatenciesQueryParameters contains the parameters required for latency metrics queries.
type LatenciesQueryParameters struct {
	BaseQueryParameters
	// Quantile is the quantile to compute from latency histogram metrics.
	// Valid range: 0 - 1 (inclusive).
	//
	// e.g. 0.99 will return the 99th percentile or P99 which is the worst latency
	// observed from 99% of all spans for the given service (and operation).
	Quantile float64
}

// CallRateQueryParameters contains the parameters required for call rate metrics queries.
type CallRateQueryParameters struct {
	BaseQueryParameters
}

// ErrorRateQueryParameters contains the parameters required for error rate metrics queries.
type ErrorRateQueryParameters struct {
	BaseQueryParameters
}
