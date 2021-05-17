// Copyright (c) 2021 The Jaeger Authors.
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

package metricsstore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
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
	// GetMinStepDuration gets the min time resolution supported by the backing metrics store,
	// e.g. 10s means the backend can only return data points that are at least 10s apart, not closer.
	GetMinStepDuration(ctx context.Context, params *MinStepDurationQueryParameters) (time.Duration, error)
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
	SpanKinds []string
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

// MinStepDurationQueryParameters contains the parameters required for fetching the minimum step duration.
type MinStepDurationQueryParameters struct{}
