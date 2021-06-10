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

package querysvc

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

type (
	// MetricsQueryService provides a means of querying R.E.D metrics from an underlying metrics store.
	MetricsQueryService interface {
		GetLatencies(ctx context.Context, params *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error)
		GetCallRates(ctx context.Context, params *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error)
		GetErrorRates(ctx context.Context, params *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error)
		GetMinStepDuration(ctx context.Context, params *metricsstore.MinStepDurationQueryParameters) (time.Duration, error)
	}

	// errMetricsQueryDisabled is the error returned by disabledMetricsQueryService.
	errMetricsQueryDisabled struct{}

	// enabledMetricsQueryService represents an "enabled" MetricsQueryService implementation.
	enabledMetricsQueryService struct {
		metricsReader metricsstore.Reader
	}

	// disabledMetricsQueryService represents a "disabled" MetricsQueryService implementation
	// where METRICS_STORAGE_TYPE has not been set.
	disabledMetricsQueryService struct{}
)

// ErrDisabled is the error returned by a "disabled" MetricsQueryService on all of its endpoints.
var ErrDisabled = &errMetricsQueryDisabled{}

// Error implements the error interface, returning the error message when metrics querying is disabled.
func (m *errMetricsQueryDisabled) Error() string {
	return "metrics querying is currently disabled"
}

// NewMetricsQueryService returns a new MetricsQueryService.
// A nil reader interface is a signal that the metrics query feature should be disabled,
// returning a disabledMetricsQueryService instance.
func NewMetricsQueryService(reader metricsstore.Reader) MetricsQueryService {
	if reader == nil {
		return &disabledMetricsQueryService{}
	}
	return &enabledMetricsQueryService{
		metricsReader: reader,
	}
}

// GetLatencies is the queryService implementation of metricsstore.Reader.
func (mqs enabledMetricsQueryService) GetLatencies(ctx context.Context, params *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return mqs.metricsReader.GetLatencies(ctx, params)
}

// GetCallRates is the queryService implementation of metricsstore.Reader.
func (mqs enabledMetricsQueryService) GetCallRates(ctx context.Context, params *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	return mqs.metricsReader.GetCallRates(ctx, params)
}

// GetErrorRates is the queryService implementation of metricsstore.Reader.
func (mqs enabledMetricsQueryService) GetErrorRates(ctx context.Context, params *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return mqs.metricsReader.GetErrorRates(ctx, params)
}

// GetMinStepDuration is the queryService implementation of metricsstore.Reader.
func (mqs enabledMetricsQueryService) GetMinStepDuration(ctx context.Context, params *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return mqs.metricsReader.GetMinStepDuration(ctx, params)
}

// GetLatencies is the queryService implementation of metricsstore.Reader.
func (mqs disabledMetricsQueryService) GetLatencies(_ context.Context, _ *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetCallRates is the queryService implementation of metricsstore.Reader.
func (mqs disabledMetricsQueryService) GetCallRates(_ context.Context, _ *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetErrorRates is the queryService implementation of metricsstore.Reader.
func (mqs disabledMetricsQueryService) GetErrorRates(_ context.Context, _ *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrDisabled
}

// GetMinStepDuration is the queryService implementation of metricsstore.Reader.
func (mqs disabledMetricsQueryService) GetMinStepDuration(_ context.Context, _ *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return 0, ErrDisabled
}
