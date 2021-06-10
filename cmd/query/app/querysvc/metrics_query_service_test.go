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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	protometrics "github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	metricsmocks "github.com/jaegertracing/jaeger/storage/metricsstore/mocks"
)

type testMetricsQueryService struct {
	queryService  MetricsQueryService
	metricsReader *metricsmocks.Reader
}

func initializeTestMetricsQueryService(enabled bool) *testMetricsQueryService {
	tqs := testMetricsQueryService{}
	var metricsReader metricsstore.Reader
	if enabled {
		mock := &metricsmocks.Reader{}
		metricsReader = mock
		tqs.metricsReader = mock
	}

	tqs.queryService = NewMetricsQueryService(metricsReader)
	return &tqs
}

// Test QueryService.GetLatencies()
func TestGetLatencies(t *testing.T) {
	tqs := initializeTestMetricsQueryService(true)
	expectedLatencies := &protometrics.MetricFamily{
		Name:    "latencies",
		Metrics: []*protometrics.Metric{},
	}
	qParams := &metricsstore.LatenciesQueryParameters{}
	tqs.metricsReader.On("GetLatencies", mock.Anything, qParams).Return(expectedLatencies, nil).Times(1)

	actualLatencies, err := tqs.queryService.GetLatencies(context.Background(), qParams)
	assert.NoError(t, err)
	assert.Equal(t, expectedLatencies, actualLatencies)
}

func TestGetLatenciesNilReader(t *testing.T) {
	tqs := initializeTestMetricsQueryService(false)
	qParams := &metricsstore.LatenciesQueryParameters{}
	r, err := tqs.queryService.GetLatencies(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
}

// Test QueryService.GetCallRates()
func TestGetCallRates(t *testing.T) {
	tqs := initializeTestMetricsQueryService(true)
	expectedCallRates := &protometrics.MetricFamily{
		Name:    "call rates",
		Metrics: []*protometrics.Metric{},
	}
	qParams := &metricsstore.CallRateQueryParameters{}
	tqs.metricsReader.On("GetCallRates", mock.Anything, qParams).Return(expectedCallRates, nil).Times(1)

	actualCallRates, err := tqs.queryService.GetCallRates(context.Background(), qParams)
	assert.NoError(t, err)
	assert.Equal(t, expectedCallRates, actualCallRates)
}

func TestGetCallRatesNilReader(t *testing.T) {
	tqs := initializeTestMetricsQueryService(false)
	qParams := &metricsstore.CallRateQueryParameters{}
	r, err := tqs.queryService.GetCallRates(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
}

// Test QueryService.GetErrorRates()
func TestGetErrorRates(t *testing.T) {
	tqs := initializeTestMetricsQueryService(true)
	expectedErrorRates := &protometrics.MetricFamily{}
	qParams := &metricsstore.ErrorRateQueryParameters{}
	tqs.metricsReader.On("GetErrorRates", mock.Anything, qParams).Return(expectedErrorRates, nil).Times(1)

	actualErrorRates, err := tqs.queryService.GetErrorRates(context.Background(), qParams)
	assert.NoError(t, err)
	assert.Equal(t, expectedErrorRates, actualErrorRates)
}

func TestGetErrorRatesNilReader(t *testing.T) {
	tqs := initializeTestMetricsQueryService(false)
	qParams := &metricsstore.ErrorRateQueryParameters{}
	r, err := tqs.queryService.GetErrorRates(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
}

// Test QueryService.GetMinStepDurations()
func TestGetMinStepDurations(t *testing.T) {
	tqs := initializeTestMetricsQueryService(true)
	expectedMinStep := time.Second
	qParams := &metricsstore.MinStepDurationQueryParameters{}
	tqs.metricsReader.On("GetMinStepDuration", mock.Anything, qParams).Return(expectedMinStep, nil).Times(1)

	actualMinStep, err := tqs.queryService.GetMinStepDuration(context.Background(), qParams)
	assert.NoError(t, err)
	assert.Equal(t, expectedMinStep, actualMinStep)
}

func TestGetMinStepDurationsNilReader(t *testing.T) {
	tqs := initializeTestMetricsQueryService(false)
	qParams := &metricsstore.MinStepDurationQueryParameters{}
	r, err := tqs.queryService.GetMinStepDuration(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
}
