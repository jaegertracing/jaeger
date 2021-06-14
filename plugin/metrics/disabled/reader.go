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

package disabled

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

type (
	// MetricsReader represents a "disabled" metricsstore.Reader implementation where
	// the METRICS_STORAGE_TYPE has not been set.
	MetricsReader struct{}

	// errMetricsQueryDisabled is the error returned by disabledMetricsQueryService.
	errMetricsQueryDisabled struct{}
)

// ErrDisabled is the error returned by a "disabled" MetricsQueryService on all of its endpoints.
var ErrDisabled = &errMetricsQueryDisabled{}

func (m *errMetricsQueryDisabled) Error() string {
	return "metrics querying is currently disabled"
}

// NewMetricsReader returns a new Disabled MetricsReader.
func NewMetricsReader() (*MetricsReader, error) {
	return &MetricsReader{}, nil
}

// GetLatencies gets the latency metrics for the given set of latency query parameters.
func (m *MetricsReader) GetLatencies(ctx context.Context, requestParams *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetCallRates gets the call rate metrics for the given set of call rate query parameters.
func (m *MetricsReader) GetCallRates(ctx context.Context, requestParams *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetErrorRates gets the error rate metrics for the given set of error rate query parameters.
func (m *MetricsReader) GetErrorRates(ctx context.Context, requestParams *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetMinStepDuration gets the minimum step duration (the smallest possible duration between two data points in a time series) supported.
func (m *MetricsReader) GetMinStepDuration(_ context.Context, _ *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return 0, ErrDisabled
}
